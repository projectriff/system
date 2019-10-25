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

package knative

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/projectriff/system/pkg/apis"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	knativev1alpha1 "github.com/projectriff/system/pkg/apis/knative/v1alpha1"
	servingv1 "github.com/projectriff/system/pkg/apis/thirdparty/knative/serving/v1"
	"github.com/projectriff/system/pkg/tracker"
)

// AdapterReconciler reconciles a Adapter object
type AdapterReconciler struct {
	client.Client
	Log     logr.Logger
	Scheme  *runtime.Scheme
	Tracker tracker.Tracker
}

// +kubebuilder:rbac:groups=knative.projectriff.io,resources=adapters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=knative.projectriff.io,resources=adapters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=build.projectriff.io,resources=applications;containers;functions,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.knative.dev,resources=configurations;services,verbs=get;list;watch;create;update;patch;delete

func (r *AdapterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("adapter", req.NamespacedName)

	var originalAdapter knativev1alpha1.Adapter
	if err := r.Get(ctx, req.NamespacedName, &originalAdapter); err != nil {
		if apierrs.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Adapter")
		return ctrl.Result{}, err
	}
	adapter := *(originalAdapter.DeepCopy())

	adapter.Default()
	adapter.Status.InitializeConditions()

	result, err := r.reconcile(ctx, log, &adapter)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(adapter.Status, originalAdapter.Status) {
		// update status
		log.Info("updating adapter status", "diff", cmp.Diff(originalAdapter.Status, adapter.Status))
		if updateErr := r.Status().Update(ctx, &adapter); updateErr != nil {
			log.Error(updateErr, "unable to update Adapter status", "adapter", adapter)
			return ctrl.Result{Requeue: true}, updateErr
		}
	}

	// return original reconcile result
	return result, err
}

func (r *AdapterReconciler) reconcile(ctx context.Context, log logr.Logger, adapter *knativev1alpha1.Adapter) (ctrl.Result, error) {
	if adapter.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	// resolve build image
	if err := r.reconcileBuildImage(ctx, log, adapter); err != nil {
		if apierrs.IsNotFound(err) {
			// we'll ignore not-found errors, since the reference build resource
			// may not exist yet.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to resolve image for Adapter", "adapter", adapter)
		return ctrl.Result{Requeue: true}, err
	}

	// reconcile configuration
	if adapter.Status.LatestImage != "" {
		if err := r.reconcileTarget(ctx, log, adapter); err != nil {
			if apierrs.IsNotFound(err) {
				// we'll ignore not-found errors, since the reference build resource
				// may not exist yet.
				return ctrl.Result{}, nil
			}
			log.Error(err, "unable to reconcile target for Adapter", "adapter", adapter)
			return ctrl.Result{Requeue: true}, err
		}
	}

	adapter.Status.ObservedGeneration = adapter.Generation

	return ctrl.Result{}, nil
}

func (r *AdapterReconciler) reconcileBuildImage(ctx context.Context, log logr.Logger, adapter *knativev1alpha1.Adapter) error {
	build := adapter.Spec.Build

	switch {
	case build.ApplicationRef != "":
		var application buildv1alpha1.Application
		key := types.NamespacedName{Namespace: adapter.Namespace, Name: build.ApplicationRef}
		// track application for new images
		r.Tracker.Track(
			tracker.NewKey(application.GetGroupVersionKind(), key),
			types.NamespacedName{Namespace: adapter.Namespace, Name: adapter.Name},
		)
		if err := r.Get(ctx, types.NamespacedName{Namespace: adapter.Namespace, Name: build.ApplicationRef}, &application); err != nil {
			return err
		}
		if application.Status.LatestImage == "" {
			return fmt.Errorf("application %q does not have a ready image", build.ApplicationRef)
		}
		adapter.Status.LatestImage = application.Status.LatestImage
		adapter.Status.MarkBuildReady()
		return nil

	case build.ContainerRef != "":
		var container buildv1alpha1.Container
		key := types.NamespacedName{Namespace: adapter.Namespace, Name: build.ContainerRef}
		// track container for new images
		r.Tracker.Track(
			tracker.NewKey(container.GetGroupVersionKind(), key),
			types.NamespacedName{Namespace: adapter.Namespace, Name: adapter.Name},
		)
		if err := r.Get(ctx, key, &container); err != nil {
			return err
		}
		if container.Status.LatestImage == "" {
			return fmt.Errorf("container %q does not have a ready image", build.ContainerRef)
		}
		adapter.Status.LatestImage = container.Status.LatestImage
		adapter.Status.MarkBuildReady()
		return nil

	case build.FunctionRef != "":
		var function buildv1alpha1.Function
		key := types.NamespacedName{Namespace: adapter.Namespace, Name: build.FunctionRef}
		// track function for new images
		r.Tracker.Track(
			tracker.NewKey(function.GetGroupVersionKind(), key),
			types.NamespacedName{Namespace: adapter.Namespace, Name: adapter.Name},
		)
		if err := r.Get(ctx, key, &function); err != nil {
			return err
		}
		if function.Status.LatestImage == "" {
			return fmt.Errorf("function %q does not have a ready image", build.FunctionRef)
		}
		adapter.Status.LatestImage = function.Status.LatestImage
		adapter.Status.MarkBuildReady()
		return nil
	}

	return fmt.Errorf("invalid adapter build")
}

func (r *AdapterReconciler) reconcileTarget(ctx context.Context, log logr.Logger, adapter *knativev1alpha1.Adapter) error {
	target := adapter.Spec.Target

	switch {
	case target.ServiceRef != "":
		var actualService servingv1.Service
		key := types.NamespacedName{Namespace: adapter.Namespace, Name: target.ServiceRef}
		// track service for changes
		r.Tracker.Track(
			tracker.NewKey(actualService.GetGroupVersionKind(), key),
			types.NamespacedName{Namespace: adapter.Namespace, Name: adapter.Name},
		)
		if err := r.Get(ctx, types.NamespacedName{Namespace: adapter.Namespace, Name: target.ServiceRef}, &actualService); err != nil {
			if errors.IsNotFound(err) {
				adapter.Status.MarkTargetNotFound("service", target.ServiceRef)
				return nil
			}
			return err
		}
		adapter.Status.MarkTargetFound()

		if actualService.Spec.Template.Spec.Containers[0].Image == adapter.Status.LatestImage {
			// already latest image
			return nil
		}

		// update service
		service := *(actualService.DeepCopy())
		service.Spec.Template.Spec.Containers[0].Image = adapter.Status.LatestImage
		log.Info("reconciling service", "diff", cmp.Diff(actualService.Spec, service.Spec))
		return r.Update(ctx, &service)

	case target.ConfigurationRef != "":
		var actualConfiguration servingv1.Configuration
		key := types.NamespacedName{Namespace: adapter.Namespace, Name: target.ConfigurationRef}
		// track configuration for changes
		r.Tracker.Track(
			tracker.NewKey(actualConfiguration.GetGroupVersionKind(), key),
			types.NamespacedName{Namespace: adapter.Namespace, Name: adapter.Name},
		)
		if err := r.Get(ctx, key, &actualConfiguration); err != nil {
			if errors.IsNotFound(err) {
				adapter.Status.MarkTargetNotFound("configuration", target.ConfigurationRef)
				return nil
			}
			return err
		}
		adapter.Status.MarkTargetFound()

		if actualConfiguration.Spec.Template.Spec.Containers[0].Image == adapter.Status.LatestImage {
			// already latest image
			return nil
		}

		// update configuration
		configuration := *(actualConfiguration.DeepCopy())
		configuration.Spec.Template.Spec.Containers[0].Image = adapter.Status.LatestImage
		log.Info("reconciling configuration", "diff", cmp.Diff(actualConfiguration.Spec, configuration.Spec))
		return r.Update(ctx, &configuration)

	}

	return fmt.Errorf("invalid adapter target")
}

func (r *AdapterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	enqueueTrackedResources := func(t apis.Resource) handler.EventHandler {
		return &handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
				requests := []reconcile.Request{}
				key := tracker.NewKey(
					t.GetGroupVersionKind(),
					types.NamespacedName{Namespace: a.Meta.GetNamespace(), Name: a.Meta.GetName()},
				)
				for _, item := range r.Tracker.Lookup(key) {
					requests = append(requests, reconcile.Request{NamespacedName: item})
				}
				return requests
			}),
		}
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&knativev1alpha1.Adapter{}).
		// watch for knative serving mutations
		Watches(&source.Kind{Type: &servingv1.Service{}}, enqueueTrackedResources(&servingv1.Service{})).
		Watches(&source.Kind{Type: &servingv1.Configuration{}}, enqueueTrackedResources(&servingv1.Configuration{})).
		// watch for build mutations
		Watches(&source.Kind{Type: &buildv1alpha1.Application{}}, enqueueTrackedResources(&buildv1alpha1.Application{})).
		Watches(&source.Kind{Type: &buildv1alpha1.Container{}}, enqueueTrackedResources(&buildv1alpha1.Container{})).
		Watches(&source.Kind{Type: &buildv1alpha1.Function{}}, enqueueTrackedResources(&buildv1alpha1.Function{})).
		Complete(r)
}
