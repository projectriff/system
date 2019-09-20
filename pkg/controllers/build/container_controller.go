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
	"strings"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
)

// ContainerReconciler reconciles a Container object
type ContainerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=build.projectriff.io,resources=containers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=build.projectriff.io,resources=containers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch

func (r *ContainerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("container", req.NamespacedName)

	var originalContainer buildv1alpha1.Container
	if err := r.Get(ctx, req.NamespacedName, &originalContainer); err != nil {
		if apierrs.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch container")
		return ctrl.Result{}, err
	}

	container := originalContainer.DeepCopy()
	container.SetDefaults(ctx)
	container.Status.InitializeConditions()

	result, err := r.reconcile(ctx, log, container)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(container.Status, originalContainer.Status) {
		// update status
		log.Info("updating container status", "container", container.Name,
			"status", container.Status)
		if updateErr := r.Status().Update(ctx, container); updateErr != nil {
			log.Error(updateErr, "unable to update Container status", "container", container)
			return ctrl.Result{Requeue: true}, updateErr
		}
	}
	return result, err
}

func (r *ContainerReconciler) reconcile(ctx context.Context, log logr.Logger, container *buildv1alpha1.Container) (ctrl.Result, error) {
	log.V(1).Info("reconciling container", "container", container.Name)
	log.V(5).Info("reconciling container", "spec", container.Spec, "labels", container.Labels)
	if container.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}
	targetImage, err := r.resolveTargetImage(ctx, container)
	if err != nil {
		if err == errMissingDefaultPrefix {
			container.Status.MarkImageDefaultPrefixMissing(err.Error())
		} else {
			container.Status.MarkImageInvalid(err.Error())
		}
		return ctrl.Result{}, err
	}
	log.V(1).Info("resolved target image", "container", container.Name, "image", targetImage)
	container.Status.TargetImage = targetImage
	// TODO: resolve the digest for the latest image
	container.Status.LatestImage = targetImage

	container.Status.MarkImageResolved()
	container.Status.ObservedGeneration = container.Generation

	return ctrl.Result{}, nil
}

func (r *ContainerReconciler) resolveTargetImage(ctx context.Context, container *buildv1alpha1.Container) (string, error) {
	if !strings.HasPrefix(container.Spec.Image, "_") {
		return container.Spec.Image, nil
	}

	var riffBuildConfig v1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Namespace: container.Namespace, Name: "riff-build"}, &riffBuildConfig); err != nil {
		if apierrs.IsNotFound(err) {
			return "", errMissingDefaultPrefix
		}
		return "", err
	}
	defaultPrefix := riffBuildConfig.Data["default-image-prefix"]
	if defaultPrefix == "" {
		return "", errMissingDefaultPrefix
	}
	image, err := buildv1alpha1.ResolveDefaultImage(container, defaultPrefix)
	if err != nil {
		return "", err
	}
	return image, nil
}

func (r *ContainerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&buildv1alpha1.Container{}).
		Complete(r)
}
