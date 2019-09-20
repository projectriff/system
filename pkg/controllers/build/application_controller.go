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

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=build.projectriff.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=build.projectriff.io,resources=applications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=build.knative.dev,resources=builds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch

func (r *ApplicationReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("application", req.NamespacedName)

	var originalApplication buildv1alpha1.Application
	if err := r.Get(ctx, req.NamespacedName, &originalApplication); err != nil {
		if apierrs.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch application")
		return ctrl.Result{}, err
	}

	application := originalApplication.DeepCopy()
	application.SetDefaults(ctx)
	application.Status.InitializeConditions()

	result, err := r.reconcile(ctx, log, application)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(application.Status, originalApplication.Status) {
		// update status
		log.Info("updating application status", "application", application.Name,
			"status", application.Status)
		if updateErr := r.Status().Update(ctx, application); updateErr != nil {
			log.Error(updateErr, "unable to update Application status", "application", application)
			return ctrl.Result{Requeue: true}, updateErr
		}
	}
	return result, err
}

func (r *ApplicationReconciler) reconcile(ctx context.Context, log logr.Logger, application *buildv1alpha1.Application) (ctrl.Result, error) {
	log.V(1).Info("reconciling application", "application", application.Name)
	log.V(5).Info("reconciling application", "spec", application.Spec, "labels", application.Labels)
	if application.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	var err error
	targetImage, err := r.resolveTargetImage(ctx, *application)
	if err != nil {
		if err == errMissingDefaultPrefix {
			application.Status.MarkImageDefaultPrefixMissing(err.Error())
		} else {
			application.Status.MarkImageInvalid(err.Error())
		}
		return ctrl.Result{}, err
	}
	log.V(1).Info("resolved target image", "application", application.Name, "image", targetImage)
	application.Status.TargetImage = targetImage

	buildName := names.ApplicationBuild(application)
	var build knbuildv1alpha1.Build
	var buildCreated bool
	if err = r.Get(ctx, types.NamespacedName{Name: buildName, Namespace: application.Namespace}, &build); err != nil {
		if apierrs.IsNotFound(err) {
			build, err = r.createBuild(ctx, log, application)
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

	if !metav1.IsControlledBy(&build, application) {
		application.Status.MarkBuildNotOwned()
		return ctrl.Result{}, fmt.Errorf("application: %q does not own Build: %q", application.Name, buildName)
	}

	if !buildCreated {
		build, err = r.reconcileBuild(ctx, log, application, build)
		if err != nil {
			return ctrl.Result{Requeue: true}, err
		}
	}

	// Update our Status based on the state of our underlying Build.
	if &build == nil {
		log.V(1).Info("marking build not used", "build", build.Name)
		application.Status.MarkBuildNotUsed()
	} else {
		log.V(1).Info("updating build status for application", "build", build.Name, "status", build.Status,
			"application", application.Name)
		application.Status.BuildName = build.Name
		application.Status.PropagateBuildStatus(&build.Status)
	}

	if application.Status.GetCondition(buildv1alpha1.ApplicationConditionBuildSucceeded).IsTrue() {
		// TODO: compute the digest of the image
		log.V(1).Info("setting application image", "application", application.Name,
			"image", targetImage)
		application.Status.LatestImage = targetImage
		application.Status.MarkImageResolved()
	}

	application.Status.ObservedGeneration = application.Generation

	return ctrl.Result{}, nil
}

func (r *ApplicationReconciler) reconcileBuild(ctx context.Context, log logr.Logger, application *buildv1alpha1.Application, build knbuildv1alpha1.Build) (knbuildv1alpha1.Build, error) {
	log.V(1).Info("reconciling build for application", "application", application.Name, "build", build.Name)
	desiredBuild := resources.MakeApplicationBuild(application)

	if buildSemanticEquals(desiredBuild, &build) {
		// No differences to reconcile.
		return build, nil
	}
	specDiff := cmp.Diff(build.Spec, desiredBuild.Spec)
	labelDiff := cmp.Diff(build.ObjectMeta.Labels, desiredBuild.ObjectMeta.Labels)
	log.Info(fmt.Sprintf("Reconciling build spec: %s\nlabels: %s\n", specDiff, labelDiff), "name", application.Name)

	existing := build.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredBuild.Spec
	existing.ObjectMeta.Labels = desiredBuild.ObjectMeta.Labels
	if err := r.Update(ctx, existing); err != nil {
		log.Error(err, "error reconciling an existing build", "build", build.Name, "application", application.Name)
		return build, err
	}
	return *existing, nil
}

func (r *ApplicationReconciler) resolveTargetImage(ctx context.Context, application buildv1alpha1.Application) (string, error) {
	if !strings.HasPrefix(application.Spec.Image, "_") {
		return application.Spec.Image, nil
	}

	var riffBuildConfig v1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Namespace: application.Namespace, Name: "riff-build"}, &riffBuildConfig); err != nil {
		if apierrs.IsNotFound(err) {
			return "", errMissingDefaultPrefix
		}
		return "", err
	}
	defaultPrefix := riffBuildConfig.Data["default-image-prefix"]
	if defaultPrefix == "" {
		return "", errMissingDefaultPrefix
	}
	image, err := buildv1alpha1.ResolveDefaultImage(&application, defaultPrefix)
	if err != nil {
		return "", err
	}
	return image, nil
}

func (r *ApplicationReconciler) createBuild(ctx context.Context, log logr.Logger, application *buildv1alpha1.Application) (knbuildv1alpha1.Build, error) {
	build := *resources.MakeApplicationBuild(application)
	log.V(1).Info("creating build for application", "namespace", application.Namespace,
		"name", application.Name, "labels", application.ObjectMeta.Labels)
	if err := ctrl.SetControllerReference(application, &build, r.Scheme); err != nil {
		return build, err
	}
	if err := r.Create(ctx, &build); err != nil {
		log.Error(err, "unable to create build", "build", build.Name)
		return build, err
	}
	return build, nil
}

func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&buildv1alpha1.Application{}).
		Owns(&knbuildv1alpha1.Build{}).
		Complete(r)
}
