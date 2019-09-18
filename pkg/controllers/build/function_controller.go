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
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	kpackbuildv1alpha1 "github.com/projectriff/system/pkg/apis/thirdparty/kpack/build/v1alpha1"
)

// FunctionReconciler reconciles a Function object
type FunctionReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=build.projectriff.io,resources=functions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=build.projectriff.io,resources=functions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=build.pivotal.io,resources=images,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch

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

	function.SetDefaults(ctx)
	function.Status.InitializeConditions()

	result, err := r.reconcile(ctx, log, &function)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(function.Status, originalFunction.Status) {
		// update status
		if updateErr := r.Status().Update(ctx, &function); updateErr != nil {
			log.Error(updateErr, "unable to update Function status", "function", function)
			return ctrl.Result{Requeue: true}, updateErr
		}
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

	// reconcile child image
	childImage, err := r.reconcileChildImage(ctx, log, function)
	if err != nil {
		log.Error(err, "unable to reconcile child Image", "function", function)
		return ctrl.Result{}, err
	}
	if childImage == nil {
		function.Status.MarkBuildNotUsed()
	} else {
		function.Status.KpackImageName = childImage.Name
		function.Status.LatestImage = childImage.Status.LatestImage
		function.Status.BuildCacheName = childImage.Status.BuildCacheName
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
	if err := r.Get(ctx, types.NamespacedName{Namespace: function.Namespace, Name: "riff-build"}, &riffBuildConfig); err != nil {
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

func (r *FunctionReconciler) reconcileChildImage(ctx context.Context, log logr.Logger, function *buildv1alpha1.Function) (*kpackbuildv1alpha1.Image, error) {
	var actualImage kpackbuildv1alpha1.Image
	if function.Status.KpackImageName != "" {
		if err := r.Get(ctx, types.NamespacedName{Namespace: function.Namespace, Name: function.Status.KpackImageName}, &actualImage); err != nil {
			log.Error(err, "unable to fetch child kpack Image")
			if !apierrs.IsNotFound(err) {
				return nil, err
			}
			// reset the KpackImageName since it no longer exists and needs to
			// be recreated
			function.Status.KpackImageName = ""
		}
		// check that the image is not controlled by another resource
		if !metav1.IsControlledBy(&actualImage, function) {
			function.Status.MarkKpackImageNotOwned()
			return nil, fmt.Errorf("Function %q does not own kpack Image %q", function.Name, actualImage.Name)
		}
	}

	desiredImage, err := r.constructImageForFunction(function)
	if err != nil {
		return nil, err
	}

	// delete image if no longer needed
	if desiredImage == nil {
		if err := r.Delete(ctx, &actualImage); err != nil {
			log.Error(err, "unable to delete kpack Image for Function", "image", actualImage)
			return nil, err
		}
		return nil, nil
	}

	// create image if it doesn't exist
	if function.Status.KpackImageName == "" {
		if err := r.Create(ctx, desiredImage); err != nil {
			log.Error(err, "unable to create kpack Image for Function", "image", desiredImage)
			return nil, err
		}
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
	if err := r.Update(ctx, image); err != nil {
		log.Error(err, "unable to update kpack Image for Function", "image", image)
		return nil, err
	}

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
			Labels:      r.constructLabelsForFunction(function),
			Annotations: make(map[string]string),
			// GenerateName: fmt.Sprintf("%s-function-", function.Name),
			Name:      fmt.Sprintf("%s-function", function.Name),
			Namespace: function.Namespace,
		},
		Spec: kpackbuildv1alpha1.ImageSpec{
			ServiceAccount: "riff-build",
			Builder: kpackbuildv1alpha1.ImageBuilder{
				TypeMeta: metav1.TypeMeta{
					Kind: "ClusterBuilder",
				},
				Name: "riff-function",
			},
			Tag:       function.Status.TargetImage,
			CacheSize: function.Spec.CacheSize,
			Source: kpackbuildv1alpha1.SourceConfig{
				// TODO add support for other types of source
				Git: &kpackbuildv1alpha1.Git{
					URL:      function.Spec.Source.Git.URL,
					Revision: function.Spec.Source.Git.Revision,
				},
				SubPath: function.Spec.Source.SubPath,
			},
			Build: kpackbuildv1alpha1.ImageBuild{
				Env: []corev1.EnvVar{
					{Name: "RIFF", Value: "true"},
					{Name: "RIFF_ARTIFACT", Value: function.Spec.Artifact},
					{Name: "RIFF_HANDLER", Value: function.Spec.Handler},
					{Name: "RIFF_OVERRIDE", Value: function.Spec.Invoker},
				},
			},
		},
	}
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&buildv1alpha1.Function{}).
		Owns(&kpackbuildv1alpha1.Image{}).
		Complete(r)
}
