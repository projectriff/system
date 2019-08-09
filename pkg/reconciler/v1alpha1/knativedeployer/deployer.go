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

package knativedeployer

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
	knativev1alpha1 "github.com/projectriff/system/pkg/apis/knative/v1alpha1"
	buildinformers "github.com/projectriff/system/pkg/client/informers/externalversions/build/v1alpha1"
	knativeinformers "github.com/projectriff/system/pkg/client/informers/externalversions/knative/v1alpha1"
	buildlisters "github.com/projectriff/system/pkg/client/listers/build/v1alpha1"
	knativelisters "github.com/projectriff/system/pkg/client/listers/knative/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/knativedeployer/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/knativedeployer/resources/names"
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
	ReconcilerName      = "Deployers"
	controllerAgentName = "deployer-controller"
)

// Reconciler implements controller.Reconciler for Deployer resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	deployerLister      knativelisters.DeployerLister
	knconfigurationLister knservinglisters.ConfigurationLister
	knrouteLister         knservinglisters.RouteLister
	applicationLister     buildlisters.ApplicationLister
	containerLister       buildlisters.ContainerLister
	functionLister        buildlisters.FunctionLister

	tracker tracker.Interface
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventdeployers to enqueue events
func NewController(
	opt reconciler.Options,
	deployerInformer knativeinformers.DeployerInformer,
	knconfigurationInformer knservinginformers.ConfigurationInformer,
	knrouteInformer knservinginformers.RouteInformer,
	applicationInformer buildinformers.ApplicationInformer,
	containerInformer buildinformers.ContainerInformer,
	functionInformer buildinformers.FunctionInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:                  reconciler.NewBase(opt, controllerAgentName),
		deployerLister:      deployerInformer.Lister(),
		knconfigurationLister: knconfigurationInformer.Lister(),
		knrouteLister:         knrouteInformer.Lister(),
		applicationLister:     applicationInformer.Lister(),
		containerLister:       containerInformer.Lister(),
		functionLister:        functionInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName)

	c.Logger.Info("Setting up event deployers")
	deployerInformer.Informer().AddEventHandler(reconciler.Handler(impl.Enqueue))

	// controlled resources
	knconfigurationInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(knativev1alpha1.SchemeGroupVersion.WithKind("Deployer")),
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})
	knrouteInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(knativev1alpha1.SchemeGroupVersion.WithKind("Deployer")),
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
	containerInformer.Informer().AddEventHandler(reconciler.Handler(
		// Call the tracker's OnChanged method, but we've seen the objects
		// coming through this path missing TypeMeta, so ensure it is properly
		// populated.
		controller.EnsureTypeMeta(
			c.tracker.OnChanged,
			buildv1alpha1.SchemeGroupVersion.WithKind("Container"),
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
// converge the two. It then updates the Status block of the Deployer resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the Deployer resource with this namespace/name
	original, err := c.deployerLister.Deployers(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("deployer %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	deployer := original.DeepCopy()

	// Reconcile this copy of the deployer and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, deployer)

	if equality.Semantic.DeepEqual(original.Status, deployer.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(deployer); uErr != nil {
		logger.Warn("Failed to update deployer status", zap.Error(uErr))
		c.Recorder.Eventf(deployer, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for Deployer %q: %v", deployer.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(deployer, corev1.EventTypeNormal, "Updated", "Updated Deployer %q", deployer.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, deployer *knativev1alpha1.Deployer) error {
	logger := logging.FromContext(ctx)
	if deployer.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	deployer.SetDefaults(context.Background())

	deployer.Status.InitializeConditions()

	if err := c.reconcileBuild(deployer); err != nil {
		return err
	}

	configurationName := resourcenames.Configuration(deployer)
	configuration, err := c.knconfigurationLister.Configurations(deployer.Namespace).Get(configurationName)
	if errors.IsNotFound(err) {
		configuration, err = c.createConfiguration(deployer)
		if err != nil {
			logger.Errorf("Failed to create Configuration %q: %v", configurationName, err)
			c.Recorder.Eventf(deployer, corev1.EventTypeWarning, "CreationFailed", "Failed to create Configuration %q: %v", configurationName, err)
			return err
		}
		if configuration != nil {
			c.Recorder.Eventf(deployer, corev1.EventTypeNormal, "Created", "Created Configuration %q", configurationName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Deployer: %q failed to Get Configuration: %q; %v", deployer.Name, configurationName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(configuration, deployer) {
		// Surface an error in the deployer's status,and return an error.
		deployer.Status.MarkConfigurationNotOwned(configurationName)
		return fmt.Errorf("Deployer: %q does not own Configuration: %q", deployer.Name, configurationName)
	} else {
		configuration, err = c.reconcileConfiguration(ctx, deployer, configuration)
		if err != nil {
			logger.Errorf("Failed to reconcile Deployer: %q failed to reconcile Configuration: %q; %v", deployer.Name, configuration, zap.Error(err))
			return err
		}
	}

	// Update our Status based on the state of our underlying Configuration.
	deployer.Status.ConfigurationName = configuration.Name
	deployer.Status.PropagateConfigurationStatus(&configuration.Status)

	routeName := resourcenames.Route(deployer)
	route, err := c.knrouteLister.Routes(deployer.Namespace).Get(routeName)
	if errors.IsNotFound(err) {
		route, err = c.createRoute(deployer)
		if err != nil {
			logger.Errorf("Failed to create Route %q: %v", routeName, err)
			c.Recorder.Eventf(deployer, corev1.EventTypeWarning, "CreationFailed", "Failed to create Route %q: %v", routeName, err)
			return err
		}
		if route != nil {
			c.Recorder.Eventf(deployer, corev1.EventTypeNormal, "Created", "Created Route %q", routeName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Deployer: %q failed to Get Route: %q; %v", deployer.Name, routeName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(route, deployer) {
		// Surface an error in the deployer's status,and return an error.
		deployer.Status.MarkRouteNotOwned(routeName)
		return fmt.Errorf("Deployer: %q does not own Route: %q", deployer.Name, routeName)
	} else if route, err = c.reconcileRoute(ctx, deployer, route); err != nil {
		logger.Errorf("Failed to reconcile Deployer: %q failed to reconcile Route: %q; %v", deployer.Name, route, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Route.
	deployer.Status.RouteName = route.Name
	deployer.Status.PropagateRouteStatus(&route.Status)

	deployer.Status.ObservedGeneration = deployer.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *knativev1alpha1.Deployer) (*knativev1alpha1.Deployer, error) {
	deployer, err := c.deployerLister.Deployers(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(deployer.Status, desired.Status) {
		return deployer, nil
	}
	becomesReady := desired.Status.IsReady() && !deployer.Status.IsReady()
	// Don't modify the informers copy.
	existing := deployer.DeepCopy()
	existing.Status = desired.Status

	h, err := c.ProjectriffClientSet.KnativeV1alpha1().Deployers(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(h.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("Deployer %q became ready after %v", deployer.Name, duration)
	}

	return h, err
}

func (c *Reconciler) reconcileBuild(deployer *knativev1alpha1.Deployer) error {
	build := deployer.Spec.Build
	if build == nil {
		return nil
	}

	switch {
	case build.ApplicationRef != "":
		application, err := c.applicationLister.Applications(deployer.Namespace).Get(build.ApplicationRef)
		if err != nil {
			return err
		}
		if application.Status.LatestImage == "" {
			return fmt.Errorf("application %q does not have a ready image", build.ApplicationRef)
		}
		deployer.Spec.Template.Containers[0].Image = application.Status.LatestImage

		// track application for new images
		gvk := buildv1alpha1.SchemeGroupVersion.WithKind("Application")
		if err := c.tracker.Track(reconciler.MakeObjectRef(application, gvk), deployer); err != nil {
			return err
		}
		return nil

	case build.ContainerRef != "":
		container, err := c.containerLister.Containers(deployer.Namespace).Get(build.ContainerRef)
		if err != nil {
			return err
		}
		if container.Status.LatestImage == "" {
			return fmt.Errorf("container %q does not have a ready image", build.ContainerRef)
		}
		deployer.Spec.Template.Containers[0].Image = container.Status.LatestImage

		// track container for new images
		gvk := buildv1alpha1.SchemeGroupVersion.WithKind("Container")
		if err := c.tracker.Track(reconciler.MakeObjectRef(container, gvk), deployer); err != nil {
			return err
		}
		return nil

	case build.FunctionRef != "":
		function, err := c.functionLister.Functions(deployer.Namespace).Get(build.FunctionRef)
		if err != nil {
			return err
		}
		if function.Status.LatestImage == "" {
			return fmt.Errorf("function %q does not have a ready image", build.FunctionRef)
		}
		deployer.Spec.Template.Containers[0].Image = function.Status.LatestImage

		// track function for new images
		gvk := buildv1alpha1.SchemeGroupVersion.WithKind("Function")
		if err := c.tracker.Track(reconciler.MakeObjectRef(function, gvk), deployer); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("invalid deployer build")
}

func (c *Reconciler) reconcileConfiguration(ctx context.Context, deployer *knativev1alpha1.Deployer, configuration *knservingv1alpha1.Configuration) (*knservingv1alpha1.Configuration, error) {
	logger := logging.FromContext(ctx)
	desiredConfiguration, err := resources.MakeConfiguration(deployer)
	if err != nil {
		return nil, err
	}

	if configurationSemanticEquals(desiredConfiguration, configuration) {
		// No differences to reconcile.
		return configuration, nil
	}
	diff, err := kmp.SafeDiff(desiredConfiguration.Spec, configuration.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff Configuration: %v", err)
	}
	logger.Infof("Reconciling configuration diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := configuration.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredConfiguration.Spec
	existing.ObjectMeta.Labels = desiredConfiguration.ObjectMeta.Labels
	return c.KnServingClientSet.ServingV1alpha1().Configurations(deployer.Namespace).Update(existing)
}

func (c *Reconciler) createConfiguration(deployer *knativev1alpha1.Deployer) (*knservingv1alpha1.Configuration, error) {
	configuration, err := resources.MakeConfiguration(deployer)
	if err != nil {
		return nil, err
	}
	return c.KnServingClientSet.ServingV1alpha1().Configurations(deployer.Namespace).Create(configuration)
}

func configurationSemanticEquals(desiredConfiguration, configuration *knservingv1alpha1.Configuration) bool {
	return equality.Semantic.DeepEqual(desiredConfiguration.Spec, configuration.Spec) &&
		equality.Semantic.DeepEqual(desiredConfiguration.ObjectMeta.Labels, configuration.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileRoute(ctx context.Context, deployer *knativev1alpha1.Deployer, route *knservingv1alpha1.Route) (*knservingv1alpha1.Route, error) {
	logger := logging.FromContext(ctx)
	desiredRoute, err := resources.MakeRoute(deployer)
	if err != nil {
		return nil, err
	}

	if routeSemanticEquals(desiredRoute, route) {
		// No differences to reconcile.
		return route, nil
	}
	diff, err := kmp.SafeDiff(desiredRoute.Spec, route.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff Route: %v", err)
	}
	logger.Infof("Reconciling route diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := route.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredRoute.Spec
	existing.ObjectMeta.Labels = desiredRoute.ObjectMeta.Labels
	return c.KnServingClientSet.ServingV1alpha1().Routes(deployer.Namespace).Update(existing)
}

func (c *Reconciler) createRoute(deployer *knativev1alpha1.Deployer) (*knservingv1alpha1.Route, error) {
	route, err := resources.MakeRoute(deployer)
	if err != nil {
		return nil, err
	}
	return c.KnServingClientSet.ServingV1alpha1().Routes(deployer.Namespace).Create(route)
}

func routeSemanticEquals(desiredRoute, route *knservingv1alpha1.Route) bool {
	return equality.Semantic.DeepEqual(desiredRoute.Spec, route.Spec) &&
		equality.Semantic.DeepEqual(desiredRoute.ObjectMeta.Labels, route.ObjectMeta.Labels)
}
