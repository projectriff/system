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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	knbuildv1alpha1 "github.com/projectriff/system/pkg/apis/thirdparty/knative/build/v1alpha1"
	"github.com/projectriff/system/pkg/controllers/build/resources"
	"github.com/projectriff/system/pkg/controllers/build/resources/names"
)

// FunctionReconciler reconciles a Function object
type FunctionReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=build.projectriff.io,resources=functions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=build.projectriff.io,resources=functions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=build.knative.dev,resources=builds,verbs=get;list;watch;create;update;patch;delete
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
		log.Error(err, "unable to fetch function")
		return ctrl.Result{}, err
	}

	function := originalFunction.DeepCopy()
	function.SetDefaults(ctx)
	function.Status.InitializeConditions()

	result, err := r.reconcile(ctx, log, function)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(function.Status, originalFunction.Status) {
		// update status
		log.Info("updating function status", "function", function.Name,
			"status", function.Status)
		if updateErr := r.Status().Update(ctx, function); updateErr != nil {
			log.Error(updateErr, "unable to update Function status", "function", function)
			return ctrl.Result{Requeue: true}, updateErr
		}
	}
	return result, err
}

func (r *FunctionReconciler) reconcile(ctx context.Context, log logr.Logger, function *buildv1alpha1.Function) (ctrl.Result, error) {
	log.V(1).Info("reconciling function", "function", function.Name)
	log.V(5).Info("reconciling function", "spec", function.Spec, "labels", function.Labels)
	if function.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	var err error
	targetImage, err := r.resolveTargetImage(ctx, function)
	if err != nil {
		if err == errMissingDefaultPrefix {
			function.Status.MarkImageDefaultPrefixMissing(err.Error())
		} else {
			function.Status.MarkImageInvalid(err.Error())
		}
		return ctrl.Result{}, err
	}
	log.V(1).Info("resolved target image", "function", function.Name, "image", targetImage)
	function.Status.TargetImage = targetImage

	buildName := names.FunctionBuild(function)
	var build knbuildv1alpha1.Build
	var buildCreated bool
	if err = r.Get(ctx, types.NamespacedName{Name: buildName, Namespace: function.Namespace}, &build); err != nil {
		if apierrs.IsNotFound(err) {
			build, err = r.createBuild(ctx, log, function)
			buildCreated = true
			if err != nil {
				log.Error(err, fmt.Sprintf("Failed to create Build %q", buildName))
				return ctrl.Result{Requeue: true}, err
			}
		} else {
			log.Error(err, fmt.Sprintf("Failed to fetch Build %q", buildName))
			return ctrl.Result{Requeue: true}, err
		}
	}

	if !metav1.IsControlledBy(&build, function) {
		function.Status.MarkBuildNotOwned()
		return ctrl.Result{}, fmt.Errorf("function: %q does not own Build: %q", function.Name, buildName)
	}

	if !buildCreated {
		build, err = r.reconcileBuild(ctx, log, function, build)
		if err != nil {
			return ctrl.Result{Requeue: true}, err
		}
	}

	// Update our Status based on the state of our underlying Build.
	if &build == nil {
		log.V(1).Info("marking build not used", "build", build.Name)
		function.Status.MarkBuildNotUsed()
	} else {
		log.V(1).Info("updating build status for function", "build", build.Name, "status", build.Status,
			"function", function.Name)
		function.Status.BuildName = build.Name
		function.Status.PropagateBuildStatus(&build.Status)
	}

	if function.Status.GetCondition(buildv1alpha1.FunctionConditionBuildSucceeded).IsTrue() {
		// TODO: compute the digest of the image
		function.Status.LatestImage = targetImage
		function.Status.MarkImageResolved()
	}

	function.Status.ObservedGeneration = function.Generation

	return ctrl.Result{}, nil
}

func (r *FunctionReconciler) reconcileBuild(ctx context.Context, log logr.Logger, function *buildv1alpha1.Function, build knbuildv1alpha1.Build) (knbuildv1alpha1.Build, error) {
	log.V(1).Info("reconciling build for function", "function", function.Name, "build", build.Name)
	desiredBuild := resources.MakeFunctionBuild(function)

	if buildSemanticEquals(desiredBuild, &build) {
		// No differences to reconcile.
		return build, nil
	}
	specDiff := cmp.Diff(build.Spec, desiredBuild.Spec)
	labelDiff := cmp.Diff(build.ObjectMeta.Labels, desiredBuild.ObjectMeta.Labels)
	log.Info(fmt.Sprintf("Reconciling build spec: %s\nlabels: %s\n", specDiff, labelDiff), "name", function.Name)

	existing := build.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredBuild.Spec
	existing.ObjectMeta.Labels = desiredBuild.ObjectMeta.Labels
	if err := r.Update(ctx, existing); err != nil {
		log.Error(err, "error reconciling an existing build", "build", build.Name, "function", function.Name)
		return build, err
	}
	return *existing, nil
}

func (r *FunctionReconciler) resolveTargetImage(ctx context.Context, function *buildv1alpha1.Function) (string, error) {
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

func (r *FunctionReconciler) createBuild(ctx context.Context, log logr.Logger, function *buildv1alpha1.Function) (knbuildv1alpha1.Build, error) {
	build := *resources.MakeFunctionBuild(function)
	log.V(1).Info("creating build for function", "namespace", function.Namespace,
		"name", function.Name, "labels", function.ObjectMeta.Labels)
	if err := ctrl.SetControllerReference(function, &build, r.Scheme); err != nil {
		return build, err
	}
	if err := r.Create(ctx, &build); err != nil {
		log.Error(err, "unable to create build", "build", build.Name)
		return build, err
	}
	return build, nil
}

func buildSemanticEquals(desiredBuild, build *knbuildv1alpha1.Build) bool {
	return equality.Semantic.DeepEqual(desiredBuild.Spec, build.Spec) &&
		equality.Semantic.DeepEqual(desiredBuild.ObjectMeta.Labels, build.ObjectMeta.Labels)
}

func (r *FunctionReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&buildv1alpha1.Function{}).
		Owns(&knbuildv1alpha1.Build{}).
		Complete(r)
}
