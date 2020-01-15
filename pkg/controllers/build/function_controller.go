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
	"reflect"
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

	imageReconciler controllers.SubReconciler
}

// +kubebuilder:rbac:groups=build.projectriff.io,resources=functions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=build.projectriff.io,resources=functions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=build.pivotal.io,resources=images,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

func (r *FunctionReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("function", req.NamespacedName)

	// TODO move to an init or constructor
	if r.imageReconciler == nil {
		r.imageReconciler = NewFunctionChildImageReconciler(r)
	}

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
	if _, err := r.imageReconciler.Reconcile(ctx, function); err != nil {
		return ctrl.Result{}, err
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

func NewFunctionChildImageReconciler(r *FunctionReconciler) *controllers.ChildReconciler {
	parentType := reflect.TypeOf(buildv1alpha1.Function{})
	childType := reflect.TypeOf(kpackbuildv1alpha1.Image{})
	childListType := reflect.TypeOf(kpackbuildv1alpha1.ImageList{})

	return &controllers.ChildReconciler{
		Client:   r.Client,
		Recorder: r.Recorder,
		Log:      r.Log.WithName(childType.Name()),
		Scheme:   r.Scheme,

		IndexField: applicationIndexField,

		ParentType:    parentType,
		ChildType:     childType,
		ChildListType: childListType,

		SemanticEquals: func(a1, a2 *kpackbuildv1alpha1.Image) bool {
			return equality.Semantic.DeepEqual(a1.Spec, a2.Spec) &&
				equality.Semantic.DeepEqual(a1.Labels, a2.Labels)
		},
		ConstructDesiredChild: func(parent *buildv1alpha1.Function) (*kpackbuildv1alpha1.Image, error) {
			if parent.Spec.Source == nil {
				return nil, nil
			}

			child := &kpackbuildv1alpha1.Image{
				ObjectMeta: metav1.ObjectMeta{
					Labels:       r.constructLabelsForFunction(parent),
					Annotations:  make(map[string]string),
					GenerateName: fmt.Sprintf("%s-function-", parent.Name),
					Namespace:    parent.Namespace,
				},
				Spec: kpackbuildv1alpha1.ImageSpec{
					Tag: parent.Status.TargetImage,
					Builder: kpackbuildv1alpha1.ImageBuilder{
						TypeMeta: metav1.TypeMeta{
							Kind: "ClusterBuilder",
						},
						Name: "riff-function",
					},
					ServiceAccount:           riffBuildServiceAccount,
					Source:                   *parent.Spec.Source,
					CacheSize:                parent.Spec.CacheSize,
					FailedBuildHistoryLimit:  parent.Spec.FailedBuildHistoryLimit,
					SuccessBuildHistoryLimit: parent.Spec.SuccessBuildHistoryLimit,
					ImageTaggingStrategy:     parent.Spec.ImageTaggingStrategy,
					Build:                    parent.Spec.Build,
				},
			}
			child.Spec.Build.Env = append(child.Spec.Build.Env,
				corev1.EnvVar{Name: "RIFF", Value: "true"},
				corev1.EnvVar{Name: "RIFF_ARTIFACT", Value: parent.Spec.Artifact},
				corev1.EnvVar{Name: "RIFF_HANDLER", Value: parent.Spec.Handler},
				corev1.EnvVar{Name: "RIFF_OVERRIDE", Value: parent.Spec.Invoker},
			)
			if err := ctrl.SetControllerReference(parent, child, r.Scheme); err != nil {
				return nil, err
			}

			return child, nil
		},
		ReflectChildStatusOnParent: func(parent *buildv1alpha1.Function, child *kpackbuildv1alpha1.Image, err error) {
			if child == nil {
				// TODO resolve to a digest
				parent.Status.LatestImage = parent.Status.TargetImage
				parent.Status.MarkBuildNotUsed()
			} else {
				parent.Status.KpackImageRef = refs.NewTypedLocalObjectReferenceForObject(child, r.Scheme)
				parent.Status.LatestImage = child.Status.LatestImage
				parent.Status.BuildCacheRef = refs.NewTypedLocalObjectReference(child.Status.BuildCacheName, schema.GroupKind{Kind: "PersistentVolumeClaim"})
				parent.Status.PropagateKpackImageStatus(&child.Status)
			}
		},
		MergeBeforeUpdate: func(current, desired *kpackbuildv1alpha1.Image) {
			current.Labels = desired.Labels
			current.Spec = desired.Spec
		},
		Sanitize: func(child *kpackbuildv1alpha1.Image) interface{} {
			return child.Spec
		},
	}
}
