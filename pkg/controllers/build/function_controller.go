/*
Copyright 2019 the original author or authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package build

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	kpackbuildv1alpha1 "github.com/projectriff/system/pkg/apis/thirdparty/kpack/build/v1alpha1"
	"github.com/projectriff/system/pkg/controllers"
	"github.com/projectriff/system/pkg/refs"
)

const functionIndexField = ".metadata.functionController"

// FunctionReconciler reconciles a Function object
type FunctionReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Log      logr.Logger
	Scheme   *runtime.Scheme
}

// +kubebuilder:rbac:groups=build.projectriff.io,resources=functions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=build.projectriff.io,resources=functions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=build.pivotal.io,resources=images,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

func (r *FunctionReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("function", req.NamespacedName)

	var originalFunction buildv1alpha1.Function
	if err := r.Get(ctx, req.NamespacedName, &originalFunction); err != nil {
		if apierrs.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Function")
		return ctrl.Result{}, err
	}
	function := *(originalFunction.DeepCopy())

	function.Default()
	function.Status.InitializeConditions()

	result, err := r.reconcile(ctx, log, &function)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(function.Status, originalFunction.Status) && function.GetDeletionTimestamp() == nil {
		// update status
		log.Info("updating function status", "diff", cmp.Diff(originalFunction.Status, function.Status))
		if updateErr := r.Status().Update(ctx, &function); updateErr != nil {
			log.Error(updateErr, "unable to update Function status", "function", function)
			r.Recorder.Eventf(&function, corev1.EventTypeWarning, "StatusUpdateFailed",
				"Failed to update status: %v", updateErr)
			return ctrl.Result{Requeue: true}, updateErr
		}
		r.Recorder.Eventf(&function, corev1.EventTypeNormal, "StatusUpdated",
			"Updated status")
	}

	// return original reconcile result
	return result, err
}

func (r *FunctionReconciler) reconcile(ctx context.Context, log logr.Logger, function *buildv1alpha1.Function) (ctrl.Result, error) {
	if function.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	// resolve target image
	targetImage, err := r.resolveTargetImage(ctx, log, function)
	if err != nil {
		if err == errMissingDefaultPrefix {
			function.Status.MarkImageDefaultPrefixMissing(err.Error())
		} else {
			function.Status.MarkImageInvalid(err.Error())
		}
		return ctrl.Result{}, err
	}
	function.Status.MarkImageResolved()
	function.Status.TargetImage = targetImage

	// reconcile child kpack image
	childImage, err := r.reconcileChildKpackImage(ctx, log, function)
	if err != nil {
		log.Error(err, "unable to reconcile child Image", "function", function)
		return ctrl.Result{}, err
	}
	if childImage == nil {
		// TODO resolve to a digest
		function.Status.LatestImage = function.Status.TargetImage
		function.Status.MarkBuildNotUsed()
	} else {
		function.Status.KpackImageRef = refs.NewTypedLocalObjectReferenceForObject(childImage, r.Scheme)
		function.Status.LatestImage = childImage.Status.LatestImage
		function.Status.BuildCacheRef = refs.NewTypedLocalObjectReference(childImage.Status.BuildCacheName, schema.GroupKind{Kind: "PersistentVolumeClaim"})
		function.Status.PropagateKpackImageStatus(&childImage.Status)
	}

	function.Status.ObservedGeneration = function.Generation

	return ctrl.Result{}, nil
}

func (r *FunctionReconciler) resolveTargetImage(ctx context.Context, log logr.Logger, function *buildv1alpha1.Function) (string, error) {
	if !strings.HasPrefix(function.Spec.Image, "_") {
		return function.Spec.Image, nil
	}

	var riffBuildConfig v1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Namespace: function.Namespace, Name: riffBuildServiceAccount}, &riffBuildConfig); err != nil {
		if apierrs.IsNotFound(err) {
			return "", errMissingDefaultPrefix
		}
		return "", err
	}
	defaultPrefix := riffBuildConfig.Data["default-image-prefix"]
	if defaultPrefix == "" {
		return "", errMissingDefaultPrefix
	}
	image, err := buildv1alpha1.ResolveDefaultImage(function, defaultPrefix)
	if err != nil {
		return "", err
	}
	return image, nil
}

func (r *FunctionReconciler) reconcileChildKpackImage(ctx context.Context, log logr.Logger, function *buildv1alpha1.Function) (*kpackbuildv1alpha1.Image, error) {
	var actualImage kpackbuildv1alpha1.Image
	var childImages kpackbuildv1alpha1.ImageList
	if err := r.List(ctx, &childImages, client.InNamespace(function.Namespace), client.MatchingField(functionIndexField, function.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childImages.Items) == 1 {
		actualImage = childImages.Items[0]
	} else if len(childImages.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraImage := range childImages.Items {
			log.Info("deleting extra kpack image", "image", extraImage)
			if err := r.Delete(ctx, &extraImage); err != nil {
				r.Recorder.Eventf(function, corev1.EventTypeWarning, "DeleteFailed",
					"Failed to delete kpack Image %q: %v", extraImage.Name, err)
				return nil, err
			}
			r.Recorder.Eventf(function, corev1.EventTypeNormal, "Deleted",
				"Deleted kpack Image %q", extraImage.Name)
		}
	}

	desiredImage, err := r.constructImageForFunction(function)
	if err != nil {
		return nil, err
	}

	if desiredImage == nil {
		// delete image if no longer needed
		if actualImage.Name != "" {
			log.Info("deleting kpack image", "image", actualImage)
			if err := r.Delete(ctx, &actualImage); err != nil {
				log.Error(err, "unable to delete kpack Image for Function", "image", actualImage)
				r.Recorder.Eventf(function, corev1.EventTypeWarning, "DeleteFailed",
					"Failed to delete kpack Image %q: %v", actualImage.Name, err)
				return nil, err
			}
			r.Recorder.Eventf(function, corev1.EventTypeNormal, "Deleted",
				"Deleted kpack Image %q", actualImage.Name)
		}
		return nil, nil
	}

	// create image if it doesn't exist
	if actualImage.Name == "" {
		log.Info("creating kpack image", "spec", desiredImage.Spec)
		if err := r.Create(ctx, desiredImage); err != nil {
			log.Error(err, "unable to create kpack Image for Function", "image", desiredImage)
			r.Recorder.Eventf(function, corev1.EventTypeWarning, "CreationFailed",
				"Failed to create kpack Image %q: %v", desiredImage.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(function, corev1.EventTypeNormal, "Created",
			"Created kpack Image %q", desiredImage.Name)
		return desiredImage, nil
	}

	if r.imageSemanticEquals(desiredImage, &actualImage) {
		// image is unchanged
		return &actualImage, nil
	}

	// update image with desired changes
	image := actualImage.DeepCopy()
	image.ObjectMeta.Labels = desiredImage.ObjectMeta.Labels
	image.Spec = desiredImage.Spec
	log.Info("reconciling kpack image", "diff", cmp.Diff(actualImage.Spec, image.Spec))
	if err := r.Update(ctx, image); err != nil {
		log.Error(err, "unable to update kpack Image for Function", "image", image)
		r.Recorder.Eventf(function, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update kpack Image %q: %v", image.Name, err)
		return nil, err
	}
	r.Recorder.Eventf(function, corev1.EventTypeNormal, "Updated",
		"Updated kpack Image %q", image.Name)

	return image, nil
}

func (r *FunctionReconciler) imageSemanticEquals(desiredImage, image *kpackbuildv1alpha1.Image) bool {
	return equality.Semantic.DeepEqual(desiredImage.Spec, image.Spec) &&
		equality.Semantic.DeepEqual(desiredImage.ObjectMeta.Labels, image.ObjectMeta.Labels)
}

func (r *FunctionReconciler) constructImageForFunction(function *buildv1alpha1.Function) (*kpackbuildv1alpha1.Image, error) {
	if function.Spec.Source == nil {
		return nil, nil
	}

	image := &kpackbuildv1alpha1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       r.constructLabelsForFunction(function),
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-function-", function.Name),
			Namespace:    function.Namespace,
		},
		Spec: kpackbuildv1alpha1.ImageSpec{
			Tag: function.Status.TargetImage,
			Builder: kpackbuildv1alpha1.ImageBuilder{
				TypeMeta: metav1.TypeMeta{
					Kind: "ClusterBuilder",
				},
				Name: "riff-function",
			},
			ServiceAccount:           riffBuildServiceAccount,
			Source:                   *function.Spec.Source,
			CacheSize:                function.Spec.CacheSize,
			FailedBuildHistoryLimit:  function.Spec.FailedBuildHistoryLimit,
			SuccessBuildHistoryLimit: function.Spec.SuccessBuildHistoryLimit,
			ImageTaggingStrategy:     function.Spec.ImageTaggingStrategy,
			Build:                    function.Spec.Build,
		},
	}
	image.Spec.Build.Env = append(image.Spec.Build.Env,
		corev1.EnvVar{Name: "RIFF", Value: "true"},
		corev1.EnvVar{Name: "RIFF_ARTIFACT", Value: function.Spec.Artifact},
		corev1.EnvVar{Name: "RIFF_HANDLER", Value: function.Spec.Handler},
		corev1.EnvVar{Name: "RIFF_OVERRIDE", Value: function.Spec.Invoker},
	)
	if err := ctrl.SetControllerReference(function, image, r.Scheme); err != nil {
		return nil, err
	}

	return image, nil
}

func (r *FunctionReconciler) constructLabelsForFunction(function *buildv1alpha1.Function) map[string]string {
	labels := make(map[string]string, len(function.ObjectMeta.Labels)+1)
	// pass through existing labels
	for k, v := range function.ObjectMeta.Labels {
		labels[k] = v
	}

	labels[buildv1alpha1.FunctionLabelKey] = function.Name

	return labels
}

func (r *FunctionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := controllers.IndexControllersOfType(mgr, functionIndexField, &buildv1alpha1.Function{}, &kpackbuildv1alpha1.Image{}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&buildv1alpha1.Function{}).
		Owns(&kpackbuildv1alpha1.Image{}).
		Complete(r)
}
