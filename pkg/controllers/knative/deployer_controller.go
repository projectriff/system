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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	"github.com/projectriff/system/pkg/controllers"
	"github.com/projectriff/system/pkg/tracker"
)

const (
	configurationIndexField = ".metadata.configurationController"
	routeIndexField         = ".metadata.routeController"
)

// DeployerReconciler reconciles a Deployer object
type DeployerReconciler struct {
	client.Client
	Log     logr.Logger
	Scheme  *runtime.Scheme
	Tracker tracker.Tracker
}

// +kubebuilder:rbac:groups=knative.projectriff.io,resources=deployers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=knative.projectriff.io,resources=deployers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=build.projectriff.io,resources=applications;containers;functions,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.knative.dev,resources=configurations;routes,verbs=get;list;watch;create;update;patch;delete

func (r *DeployerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("deployer", req.NamespacedName)

	var originalDeployer knativev1alpha1.Deployer
	if err := r.Get(ctx, req.NamespacedName, &originalDeployer); err != nil {
		if apierrs.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Deployer")
		return ctrl.Result{}, err
	}
	deployer := *(originalDeployer.DeepCopy())

	deployer.Default()
	deployer.Status.InitializeConditions()

	result, err := r.reconcile(ctx, log, &deployer)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(deployer.Status, originalDeployer.Status) {
		// update status
		log.Info("updating deployer status", "diff", cmp.Diff(originalDeployer.Status, deployer.Status))
		if updateErr := r.Status().Update(ctx, &deployer); updateErr != nil {
			log.Error(updateErr, "unable to update Deployer status", "deployer", deployer)
			return ctrl.Result{Requeue: true}, updateErr
		}
	}

	// return original reconcile result
	return result, err
}

func (r *DeployerReconciler) reconcile(ctx context.Context, log logr.Logger, deployer *knativev1alpha1.Deployer) (ctrl.Result, error) {
	if deployer.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	// resolve build image
	if err := r.reconcileBuildImage(ctx, log, deployer); err != nil {
		if apierrs.IsNotFound(err) {
			// we'll ignore not-found errors, since the reference build resource
			// may not exist yet.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to resolve image for Deployer", "deployer", deployer)
		return ctrl.Result{Requeue: true}, err
	}

	// reconcile configuration
	childConfiguration, err := r.reconcileChildConfiguration(ctx, log, deployer)
	if err != nil {
		log.Error(err, "unable to reconcile child Configuration", "deployer", deployer)
		return ctrl.Result{}, err
	}
	deployer.Status.ConfigurationName = childConfiguration.Name
	deployer.Status.PropagateConfigurationStatus(&childConfiguration.Status)

	// reconcile route
	childRoute, err := r.reconcileChildRoute(ctx, log, deployer)
	if err != nil {
		log.Error(err, "unable to reconcile child Route", "deployer", deployer)
		return ctrl.Result{}, err
	}
	deployer.Status.RouteName = childRoute.Name
	deployer.Status.PropagateRouteStatus(&childRoute.Status)

	deployer.Status.ObservedGeneration = deployer.Generation

	return ctrl.Result{}, nil
}

func (r *DeployerReconciler) reconcileBuildImage(ctx context.Context, log logr.Logger, deployer *knativev1alpha1.Deployer) error {
	build := deployer.Spec.Build
	if build == nil {
		deployer.Status.LatestImage = deployer.Spec.Template.Containers[0].Image
		return nil
	}

	switch {
	case build.ApplicationRef != "":
		var application buildv1alpha1.Application
		key := types.NamespacedName{Namespace: deployer.Namespace, Name: build.ApplicationRef}
		// track application for new images
		r.Tracker.Track(
			tracker.NewKey(application.GetGroupVersionKind(), key),
			types.NamespacedName{Namespace: deployer.Namespace, Name: deployer.Name},
		)
		if err := r.Get(ctx, key, &application); err != nil {
			return err
		}
		if application.Status.LatestImage == "" {
			return fmt.Errorf("application %q does not have a ready image", build.ApplicationRef)
		}
		deployer.Status.LatestImage = application.Status.LatestImage
		return nil

	case build.ContainerRef != "":
		var container buildv1alpha1.Container
		key := types.NamespacedName{Namespace: deployer.Namespace, Name: build.ContainerRef}
		// track container for new images
		r.Tracker.Track(
			tracker.NewKey(container.GetGroupVersionKind(), key),
			types.NamespacedName{Namespace: deployer.Namespace, Name: deployer.Name},
		)
		if err := r.Get(ctx, key, &container); err != nil {
			return err
		}
		if container.Status.LatestImage == "" {
			return fmt.Errorf("container %q does not have a ready image", build.ContainerRef)
		}
		deployer.Status.LatestImage = container.Status.LatestImage
		return nil

	case build.FunctionRef != "":
		var function buildv1alpha1.Function
		key := types.NamespacedName{Namespace: deployer.Namespace, Name: build.FunctionRef}
		// track function for new images
		r.Tracker.Track(
			tracker.NewKey(function.GetGroupVersionKind(), key),
			types.NamespacedName{Namespace: deployer.Namespace, Name: deployer.Name},
		)
		if err := r.Get(ctx, key, &function); err != nil {
			return err
		}
		if function.Status.LatestImage == "" {
			return fmt.Errorf("function %q does not have a ready image", build.FunctionRef)
		}
		deployer.Status.LatestImage = function.Status.LatestImage
		return nil

	}

	return fmt.Errorf("invalid deployer build")
}

func (r *DeployerReconciler) reconcileChildConfiguration(ctx context.Context, log logr.Logger, deployer *knativev1alpha1.Deployer) (*servingv1.Configuration, error) {
	var actualConfiguration servingv1.Configuration
	var childConfigurations servingv1.ConfigurationList
	if err := r.List(ctx, &childConfigurations, client.InNamespace(deployer.Namespace), client.MatchingField(configurationIndexField, deployer.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childConfigurations.Items) == 1 {
		actualConfiguration = childConfigurations.Items[0]
	} else if len(childConfigurations.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraConfiguration := range childConfigurations.Items {
			log.Info("deleting extra configuration", "configuration", extraConfiguration)
			if err := r.Delete(ctx, &extraConfiguration); err != nil {
				return nil, err
			}
		}
	}

	desiredConfiguration, err := r.constructConfigurationForDeployer(deployer)
	if err != nil {
		return nil, err
	}

	// delete configuration if no longer needed
	if desiredConfiguration == nil {
		log.Info("deleting configuration", "configuration", actualConfiguration)
		if err := r.Delete(ctx, &actualConfiguration); err != nil {
			log.Error(err, "unable to delete Configuration for Deployer", "configuration", actualConfiguration)
			return nil, err
		}
		return nil, nil
	}

	// create configuration if it doesn't exist
	if actualConfiguration.Name == "" {
		log.Info("creating configuration", "spec", desiredConfiguration.Spec)
		if err := r.Create(ctx, desiredConfiguration); err != nil {
			log.Error(err, "unable to create Configuration for Deployer", "configuration", desiredConfiguration)
			return nil, err
		}
		return desiredConfiguration, nil
	}

	if r.configurationSemanticEquals(desiredConfiguration, &actualConfiguration) {
		// configuration is unchanged
		return &actualConfiguration, nil
	}

	// update configuration with desired changes
	configuration := actualConfiguration.DeepCopy()
	configuration.ObjectMeta.Labels = desiredConfiguration.ObjectMeta.Labels
	configuration.Spec = desiredConfiguration.Spec
	log.Info("reconciling configuration", "diff", cmp.Diff(actualConfiguration.Spec, configuration.Spec))
	if err := r.Update(ctx, configuration); err != nil {
		log.Error(err, "unable to update Configuration for Deployer", "configuration", configuration)
		return nil, err
	}

	return configuration, nil
}

func (r *DeployerReconciler) configurationSemanticEquals(desiredConfiguration, configuration *servingv1.Configuration) bool {
	return equality.Semantic.DeepEqual(desiredConfiguration.Spec, configuration.Spec) &&
		equality.Semantic.DeepEqual(desiredConfiguration.ObjectMeta.Labels, configuration.ObjectMeta.Labels)
}

func (r *DeployerReconciler) constructConfigurationForDeployer(deployer *knativev1alpha1.Deployer) (*servingv1.Configuration, error) {
	labels := r.constructLabelsForDeployer(deployer)

	configuration := &servingv1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-deployer-", deployer.Name),
			Namespace:    deployer.Namespace,
			Labels:       labels,
		},
		Spec: servingv1.ConfigurationSpec{
			Template: servingv1.RevisionTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: servingv1.RevisionSpec{
					PodSpec: corev1.PodSpec{
						ServiceAccountName: deployer.Spec.Template.ServiceAccountName,
						Containers:         deployer.Spec.Template.Containers,
						Volumes:            deployer.Spec.Template.Volumes,
					},
				},
			},
		},
	}
	if configuration.Spec.Template.Spec.Containers[0].Image == "" {
		configuration.Spec.Template.Spec.Containers[0].Image = deployer.Status.LatestImage
	}
	if err := ctrl.SetControllerReference(deployer, configuration, r.Scheme); err != nil {
		return nil, err
	}

	return configuration, nil
}

func (r *DeployerReconciler) reconcileChildRoute(ctx context.Context, log logr.Logger, deployer *knativev1alpha1.Deployer) (*servingv1.Route, error) {
	var actualRoute servingv1.Route
	var childRoutes servingv1.RouteList
	if err := r.List(ctx, &childRoutes, client.InNamespace(deployer.Namespace), client.MatchingField(routeIndexField, deployer.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childRoutes.Items) == 1 {
		actualRoute = childRoutes.Items[0]
	} else if len(childRoutes.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraRoute := range childRoutes.Items {
			log.Info("deleting extra route", "route", extraRoute)
			if err := r.Delete(ctx, &extraRoute); err != nil {
				return nil, err
			}
		}
	}

	desiredRoute, err := r.constructRouteForDeployer(deployer)
	if err != nil {
		return nil, err
	}

	// delete route if no longer needed
	if desiredRoute == nil {
		log.Info("deleting route", "route", actualRoute)
		if err := r.Delete(ctx, &actualRoute); err != nil {
			log.Error(err, "unable to delete Route for Deployer", "route", actualRoute)
			return nil, err
		}
		return nil, nil
	}

	// create route if it doesn't exist
	if actualRoute.Name == "" {
		log.Info("creating route", "spec", desiredRoute.Spec)
		if err := r.Create(ctx, desiredRoute); err != nil {
			log.Error(err, "unable to create Route for Deployer", "route", desiredRoute)
			return nil, err
		}
		return desiredRoute, nil
	}

	if r.routeSemanticEquals(desiredRoute, &actualRoute) {
		// route is unchanged
		return &actualRoute, nil
	}

	// update route with desired changes
	route := actualRoute.DeepCopy()
	route.ObjectMeta.Labels = desiredRoute.ObjectMeta.Labels
	route.Spec = desiredRoute.Spec
	log.Info("reconciling route", "diff", cmp.Diff(actualRoute.Spec, route.Spec))
	if err := r.Update(ctx, route); err != nil {
		log.Error(err, "unable to update Route for Deployer", "configuration", route)
		return nil, err
	}

	return route, nil
}

func (r *DeployerReconciler) routeSemanticEquals(desiredRoute, route *servingv1.Route) bool {
	return equality.Semantic.DeepEqual(desiredRoute.Spec, route.Spec) &&
		equality.Semantic.DeepEqual(desiredRoute.ObjectMeta.Labels, route.ObjectMeta.Labels)
}

func (r *DeployerReconciler) constructRouteForDeployer(deployer *knativev1alpha1.Deployer) (*servingv1.Route, error) {
	if deployer.Status.ConfigurationName == "" {
		return nil, fmt.Errorf("unable to create Route, waiting for Configuration")
	}

	labels := r.constructLabelsForDeployer(deployer)
	var allTraffic int64 = 100

	route := &servingv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-deployer-", deployer.Name),
			Namespace:    deployer.Namespace,
		},
		Spec: servingv1.RouteSpec{
			Traffic: []servingv1.TrafficTarget{
				{
					Percent:           &allTraffic,
					ConfigurationName: deployer.Status.ConfigurationName,
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(deployer, route, r.Scheme); err != nil {
		return nil, err
	}

	return route, nil
}

func (r *DeployerReconciler) constructLabelsForDeployer(deployer *knativev1alpha1.Deployer) map[string]string {
	labels := make(map[string]string, len(deployer.ObjectMeta.Labels)+1)
	// pass through existing labels
	for k, v := range deployer.ObjectMeta.Labels {
		labels[k] = v
	}

	labels[knativev1alpha1.DeployerLabelKey] = deployer.Name

	return labels
}

func (r *DeployerReconciler) SetupWithManager(mgr ctrl.Manager) error {
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

	if err := controllers.IndexControllersOfType(mgr, configurationIndexField, &knativev1alpha1.Deployer{}, &servingv1.Configuration{}); err != nil {
		return err
	}
	if err := controllers.IndexControllersOfType(mgr, routeIndexField, &knativev1alpha1.Deployer{}, &servingv1.Route{}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&knativev1alpha1.Deployer{}).
		Owns(&servingv1.Configuration{}).
		Owns(&servingv1.Route{}).
		// watch for build mutations to update dependent deployers
		Watches(&source.Kind{Type: &buildv1alpha1.Application{}}, enqueueTrackedResources(&buildv1alpha1.Application{})).
		Watches(&source.Kind{Type: &buildv1alpha1.Container{}}, enqueueTrackedResources(&buildv1alpha1.Container{})).
		Watches(&source.Kind{Type: &buildv1alpha1.Function{}}, enqueueTrackedResources(&buildv1alpha1.Function{})).
		Complete(r)
}
