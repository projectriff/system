/*
Copyright 2018 The Knative Authors

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

package route

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/kmp"
	"github.com/knative/pkg/logging"
	"github.com/knative/pkg/tracker"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	knservinginformers "github.com/knative/serving/pkg/client/informers/externalversions/serving/v1alpha1"
	knservinglisters "github.com/knative/serving/pkg/client/listers/serving/v1alpha1"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	requestv1alpha1 "github.com/projectriff/system/pkg/apis/request/v1alpha1"
	buildinformers "github.com/projectriff/system/pkg/client/informers/externalversions/build/v1alpha1"
	requestinformers "github.com/projectriff/system/pkg/client/informers/externalversions/request/v1alpha1"
	buildlisters "github.com/projectriff/system/pkg/client/listers/build/v1alpha1"
	requestlisters "github.com/projectriff/system/pkg/client/listers/request/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/route/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/route/resources/names"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	// ReconcilerName is the name of the reconciler
	ReconcilerName      = "Routes"
	controllerAgentName = "route-controller"
)

// Reconciler implements controller.Reconciler for Route resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	routeLister           requestlisters.RouteLister
	knconfigurationLister knservinglisters.ConfigurationLister
	knrouteLister         knservinglisters.RouteLister
	applicationLister     buildlisters.ApplicationLister
	functionLister        buildlisters.FunctionLister

	tracker tracker.Interface
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	routeInformer requestinformers.RouteInformer,
	knconfigurationInformer knservinginformers.ConfigurationInformer,
	knrouteInformer knservinginformers.RouteInformer,
	applicationInformer buildinformers.ApplicationInformer,
	functionInformer buildinformers.FunctionInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:                  reconciler.NewBase(opt, controllerAgentName),
		routeLister:           routeInformer.Lister(),
		knconfigurationLister: knconfigurationInformer.Lister(),
		knrouteLister:         knrouteInformer.Lister(),
		applicationLister:     applicationInformer.Lister(),
		functionLister:        functionInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	routeInformer.Informer().AddEventHandler(reconciler.Handler(impl.Enqueue))

	// controlled resources
	knconfigurationInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(requestv1alpha1.SchemeGroupVersion.WithKind("Route")),
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})
	knrouteInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(requestv1alpha1.SchemeGroupVersion.WithKind("Route")),
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})

	// referenced resources
	c.tracker = tracker.New(impl.EnqueueKey, opt.GetTrackerLease())
	applicationInformer.Informer().AddEventHandler(reconciler.Handler(
		// Call the tracker's OnChanged method, but we've seen the objects
		// coming through this path missing TypeMeta, so ensure it is properly
		// populated.
		controller.EnsureTypeMeta(
			c.tracker.OnChanged,
			buildv1alpha1.SchemeGroupVersion.WithKind("Application"),
		),
	))
	functionInformer.Informer().AddEventHandler(reconciler.Handler(
		// Call the tracker's OnChanged method, but we've seen the objects
		// coming through this path missing TypeMeta, so ensure it is properly
		// populated.
		controller.EnsureTypeMeta(
			c.tracker.OnChanged,
			buildv1alpha1.SchemeGroupVersion.WithKind("Function"),
		),
	))

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Route resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the Route resource with this namespace/name
	original, err := c.routeLister.Routes(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("route %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	route := original.DeepCopy()

	// Reconcile this copy of the route and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, route)

	if equality.Semantic.DeepEqual(original.Status, route.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(route); uErr != nil {
		logger.Warn("Failed to update route status", zap.Error(uErr))
		c.Recorder.Eventf(route, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for Route %q: %v", route.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(route, corev1.EventTypeNormal, "Updated", "Updated Route %q", route.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, route *requestv1alpha1.Route) error {
	logger := logging.FromContext(ctx)
	if route.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	route.SetDefaults(context.Background())

	route.Status.InitializeConditions()

	if err := c.reconcileBuild(route); err != nil {
		return err
	}

	knconfigurationName := resourcenames.KnConfiguration(route)
	knconfiguration, err := c.knconfigurationLister.Configurations(route.Namespace).Get(knconfigurationName)
	if errors.IsNotFound(err) {
		knconfiguration, err = c.createKnConfiguration(route)
		if err != nil {
			logger.Errorf("Failed to create Configuration %q: %v", knconfigurationName, err)
			c.Recorder.Eventf(route, corev1.EventTypeWarning, "CreationFailed", "Failed to create Configuration %q: %v", knconfigurationName, err)
			return err
		}
		if knconfiguration != nil {
			c.Recorder.Eventf(route, corev1.EventTypeNormal, "Created", "Created Configuration %q", knconfigurationName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Route: %q failed to Get Configuration: %q; %v", route.Name, knconfigurationName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(knconfiguration, route) {
		// Surface an error in the route's status, and return an error.
		route.Status.MarkKnConfigurationNotOwned(knconfigurationName)
		return fmt.Errorf("Route: %q does not own Configuration: %q", route.Name, knconfigurationName)
	} else {
		knconfiguration, err = c.reconcileKnConfiguration(ctx, route, knconfiguration)
		if err != nil {
			logger.Errorf("Failed to reconcile Route: %q failed to reconcile Configuration: %q; %v", route.Name, knconfiguration, zap.Error(err))
			return err
		}
	}

	// Update our Status based on the state of our underlying Configuration.
	route.Status.KnConfigurationName = knconfiguration.Name
	route.Status.PropagateKnConfigurationStatus(&knconfiguration.Status)

	knrouteName := resourcenames.KnRoute(route)
	knroute, err := c.knrouteLister.Routes(route.Namespace).Get(knrouteName)
	if errors.IsNotFound(err) {
		knroute, err = c.createKnRoute(route)
		if err != nil {
			logger.Errorf("Failed to create Route %q: %v", knrouteName, err)
			c.Recorder.Eventf(route, corev1.EventTypeWarning, "CreationFailed", "Failed to create Route %q: %v", knrouteName, err)
			return err
		}
		if knroute != nil {
			c.Recorder.Eventf(route, corev1.EventTypeNormal, "Created", "Created Route %q", knrouteName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Route: %q failed to Get Route: %q; %v", route.Name, knrouteName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(knroute, route) {
		// Surface an error in the route's status, and return an error.
		route.Status.MarkKnRouteNotOwned(knrouteName)
		return fmt.Errorf("Route: %q does not own Route: %q", route.Name, knrouteName)
	} else if knroute, err = c.reconcileKnRoute(ctx, route, knroute); err != nil {
		logger.Errorf("Failed to reconcile Route: %q failed to reconcile Route: %q; %v", route.Name, knroute, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Route.
	route.Status.KnRouteName = knroute.Name
	route.Status.PropagateKnRouteStatus(&knroute.Status)

	route.Status.ObservedGeneration = route.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *requestv1alpha1.Route) (*requestv1alpha1.Route, error) {
	route, err := c.routeLister.Routes(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(route.Status, desired.Status) {
		return route, nil
	}
	becomesReady := desired.Status.IsReady() && !route.Status.IsReady()
	// Don't modify the informers copy.
	existing := route.DeepCopy()
	existing.Status = desired.Status

	h, err := c.ProjectriffClientSet.RequestV1alpha1().Routes(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(h.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("Route %q became ready after %v", route.Name, duration)
	}

	return h, err
}

func (c *Reconciler) reconcileBuild(route *requestv1alpha1.Route) error {
	build := route.Spec.Build
	if build == nil {
		return nil
	}

	switch {
	case build.ApplicationRef != "":
		application, err := c.applicationLister.Applications(route.Namespace).Get(build.ApplicationRef)
		if err != nil {
			return err
		}
		if application.Status.LatestImage == "" {
			return fmt.Errorf("application %q does not have a ready image", build.ApplicationRef)
		}
		route.Spec.Template.Containers[0].Image = application.Status.LatestImage

		// track application for new images
		gvk := buildv1alpha1.SchemeGroupVersion.WithKind("Application")
		if err := c.tracker.Track(reconciler.MakeObjectRef(application, gvk), route); err != nil {
			return err
		}
		return nil

	case build.FunctionRef != "":
		function, err := c.functionLister.Functions(route.Namespace).Get(build.FunctionRef)
		if err != nil {
			return err
		}
		if function.Status.LatestImage == "" {
			return fmt.Errorf("function %q does not have a ready image", build.FunctionRef)
		}
		route.Spec.Template.Containers[0].Image = function.Status.LatestImage

		// track function for new images
		gvk := buildv1alpha1.SchemeGroupVersion.WithKind("Function")
		if err := c.tracker.Track(reconciler.MakeObjectRef(function, gvk), route); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("invalid route build")
}

func (c *Reconciler) reconcileKnConfiguration(ctx context.Context, route *requestv1alpha1.Route, knconfiguration *knservingv1alpha1.Configuration) (*knservingv1alpha1.Configuration, error) {
	logger := logging.FromContext(ctx)
	desiredKnConfiguration, err := resources.MakeKnConfiguration(route)
	if err != nil {
		return nil, err
	}

	if knconfigurationSemanticEquals(desiredKnConfiguration, knconfiguration) {
		// No differences to reconcile.
		return knconfiguration, nil
	}
	diff, err := kmp.SafeDiff(desiredKnConfiguration.Spec, knconfiguration.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff Configuration: %v", err)
	}
	logger.Infof("Reconciling configuration diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := knconfiguration.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredKnConfiguration.Spec
	existing.ObjectMeta.Labels = desiredKnConfiguration.ObjectMeta.Labels
	return c.KnServingClientSet.ServingV1alpha1().Configurations(route.Namespace).Update(existing)
}

func (c *Reconciler) createKnConfiguration(route *requestv1alpha1.Route) (*knservingv1alpha1.Configuration, error) {
	knconfiguration, err := resources.MakeKnConfiguration(route)
	if err != nil {
		return nil, err
	}
	return c.KnServingClientSet.ServingV1alpha1().Configurations(route.Namespace).Create(knconfiguration)
}

func knconfigurationSemanticEquals(desiredKnConfiguration, knconfiguration *knservingv1alpha1.Configuration) bool {
	return equality.Semantic.DeepEqual(desiredKnConfiguration.Spec, knconfiguration.Spec) &&
		equality.Semantic.DeepEqual(desiredKnConfiguration.ObjectMeta.Labels, knconfiguration.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileKnRoute(ctx context.Context, route *requestv1alpha1.Route, knroute *knservingv1alpha1.Route) (*knservingv1alpha1.Route, error) {
	logger := logging.FromContext(ctx)
	desiredKnRoute, err := resources.MakeKnRoute(route)
	if err != nil {
		return nil, err
	}

	if knrouteSemanticEquals(desiredKnRoute, knroute) {
		// No differences to reconcile.
		return knroute, nil
	}
	diff, err := kmp.SafeDiff(desiredKnRoute.Spec, knroute.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff Route: %v", err)
	}
	logger.Infof("Reconciling route diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := knroute.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredKnRoute.Spec
	existing.ObjectMeta.Labels = desiredKnRoute.ObjectMeta.Labels
	return c.KnServingClientSet.ServingV1alpha1().Routes(route.Namespace).Update(existing)
}

func (c *Reconciler) createKnRoute(route *requestv1alpha1.Route) (*knservingv1alpha1.Route, error) {
	knroute, err := resources.MakeKnRoute(route)
	if err != nil {
		return nil, err
	}
	return c.KnServingClientSet.ServingV1alpha1().Routes(route.Namespace).Create(knroute)
}

func knrouteSemanticEquals(desiredKnRoute, knroute *knservingv1alpha1.Route) bool {
	return equality.Semantic.DeepEqual(desiredKnRoute.Spec, knroute.Spec) &&
		equality.Semantic.DeepEqual(desiredKnRoute.ObjectMeta.Labels, knroute.ObjectMeta.Labels)
}
