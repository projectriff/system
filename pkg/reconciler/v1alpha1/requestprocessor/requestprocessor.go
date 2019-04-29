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

package requestprocessor

import (
	"context"
	"fmt"
	"reflect"
	"time"

	knduckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/kmeta"
	"github.com/knative/pkg/kmp"
	"github.com/knative/pkg/logging"
	"github.com/knative/pkg/tracker"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	knservinginformers "github.com/knative/serving/pkg/client/informers/externalversions/serving/v1alpha1"
	knservinglisters "github.com/knative/serving/pkg/client/listers/serving/v1alpha1"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	runv1alpha1 "github.com/projectriff/system/pkg/apis/run/v1alpha1"
	buildinformers "github.com/projectriff/system/pkg/client/informers/externalversions/build/v1alpha1"
	runinformers "github.com/projectriff/system/pkg/client/informers/externalversions/run/v1alpha1"
	buildlisters "github.com/projectriff/system/pkg/client/listers/build/v1alpha1"
	runlisters "github.com/projectriff/system/pkg/client/listers/run/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/requestprocessor/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/requestprocessor/resources/names"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
)

const (
	// ReconcilerName is the name of the reconciler
	ReconcilerName      = "RequestProcessors"
	controllerAgentName = "requestprocessor-controller"
)

// Reconciler implements controller.Reconciler for RequestProcessor resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	requestprocessorLister runlisters.RequestProcessorLister
	knconfigurationLister  knservinglisters.ConfigurationLister
	knrouteLister          knservinglisters.RouteLister
	applicationLister      buildlisters.ApplicationLister
	functionLister         buildlisters.FunctionLister

	tracker tracker.Interface
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	requestprocessorInformer runinformers.RequestProcessorInformer,
	knconfigurationInformer knservinginformers.ConfigurationInformer,
	knrouteInformer knservinginformers.RouteInformer,
	applicationInformer buildinformers.ApplicationInformer,
	functionInformer buildinformers.FunctionInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:                   reconciler.NewBase(opt, controllerAgentName),
		requestprocessorLister: requestprocessorInformer.Lister(),
		knconfigurationLister:  knconfigurationInformer.Lister(),
		knrouteLister:          knrouteInformer.Lister(),
		applicationLister:      applicationInformer.Lister(),
		functionLister:         functionInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	requestprocessorInformer.Informer().AddEventHandler(reconciler.Handler(impl.Enqueue))

	// controlled resources
	knconfigurationInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(runv1alpha1.SchemeGroupVersion.WithKind("RequestProcessor")),
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})
	knrouteInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(runv1alpha1.SchemeGroupVersion.WithKind("RequestProcessor")),
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})
	applicationInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(runv1alpha1.SchemeGroupVersion.WithKind("RequestProcessor")),
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})
	functionInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(runv1alpha1.SchemeGroupVersion.WithKind("RequestProcessor")),
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
// converge the two. It then updates the Status block of the RequestProcessor resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the RequestProcessor resource with this namespace/name
	original, err := c.requestprocessorLister.RequestProcessors(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("requestprocessor %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	requestprocessor := original.DeepCopy()

	// Reconcile this copy of the requestprocessor and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, requestprocessor)

	if equality.Semantic.DeepEqual(original.Status, requestprocessor.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(requestprocessor); uErr != nil {
		logger.Warn("Failed to update requestprocessor status", zap.Error(uErr))
		c.Recorder.Eventf(requestprocessor, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for RequestProcessor %q: %v", requestprocessor.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(requestprocessor, corev1.EventTypeNormal, "Updated", "Updated RequestProcessor %q", requestprocessor.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, requestprocessor *runv1alpha1.RequestProcessor) error {
	logger := logging.FromContext(ctx)
	if requestprocessor.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	requestprocessor.SetDefaults(context.Background())
	// Default percentages before processing, but unlike SetDefaults are not persisted
	// by the webhook.
	requestprocessor.Spec.SetDefaultPercents(context.Background())

	requestprocessor.Status.InitializeConditions()

	builds, err := c.reconcileBuilds(ctx, requestprocessor)
	if err != nil {
		return err
	}

	// Update our Status based on the state of our underlying builds.
	requestprocessor.Status.Builds = builds
	requestprocessor.Status.MarkBuildsCreated()

	configurations, err := c.reconcileConfigurations(ctx, requestprocessor)
	if err != nil {
		return err
	}

	// Update our Status based on the state of our underlying Configurations.
	requestprocessor.Status.ConfigurationNames = make([]string, len(configurations))
	var configurationConditionFalse *knduckv1alpha1.Condition
	var configurationConditionUnknown *knduckv1alpha1.Condition
	for i, configuration := range configurations {
		requestprocessor.Status.ConfigurationNames[i] = configurations[i].Name
		sc := configuration.Status.GetCondition(knservingv1alpha1.ConfigurationConditionReady)
		if sc == nil {
			continue
		}
		switch {
		case sc.Status == corev1.ConditionFalse:
			configurationConditionFalse = sc
		case sc.Status == corev1.ConditionUnknown:
			configurationConditionUnknown = sc
		}
	}
	if configurationConditionFalse != nil {
		requestprocessor.Status.MarkConfigurationsFailed(configurationConditionFalse.Reason, configurationConditionFalse.Message)
	} else if configurationConditionUnknown != nil {
		requestprocessor.Status.MarkConfigurationsUnknown(configurationConditionUnknown.Reason, configurationConditionUnknown.Message)
	} else {
		requestprocessor.Status.MarkConfigurationsReady()
	}

	routeName := resourcenames.Route(requestprocessor)
	route, err := c.knrouteLister.Routes(requestprocessor.Namespace).Get(routeName)
	if errors.IsNotFound(err) {
		route, err = c.createRoute(requestprocessor)
		if err != nil {
			logger.Errorf("Failed to create Route %q: %v", routeName, err)
			c.Recorder.Eventf(requestprocessor, corev1.EventTypeWarning, "CreationFailed", "Failed to create Route %q: %v", routeName, err)
			return err
		}
		if route != nil {
			c.Recorder.Eventf(requestprocessor, corev1.EventTypeNormal, "Created", "Created Route %q", routeName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile RequestProcessor: %q failed to Get Route: %q; %v", requestprocessor.Name, routeName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(route, requestprocessor) {
		// Surface an error in the requestprocessor's status,and return an error.
		requestprocessor.Status.MarkRouteNotOwned(routeName)
		return fmt.Errorf("RequestProcessor: %q does not own Route: %q", requestprocessor.Name, routeName)
	} else if route, err = c.reconcileRoute(ctx, requestprocessor, route); err != nil {
		logger.Errorf("Failed to reconcile RequestProcessor: %q failed to reconcile Route: %q; %v", requestprocessor.Name, route, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Route.
	requestprocessor.Status.RouteName = route.Name
	requestprocessor.Status.PropagateRouteStatus(&route.Status)

	requestprocessor.Status.ObservedGeneration = requestprocessor.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *runv1alpha1.RequestProcessor) (*runv1alpha1.RequestProcessor, error) {
	requestprocessor, err := c.requestprocessorLister.RequestProcessors(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(requestprocessor.Status, desired.Status) {
		return requestprocessor, nil
	}
	becomesReady := desired.Status.IsReady() && !requestprocessor.Status.IsReady()
	// Don't modify the informers copy.
	existing := requestprocessor.DeepCopy()
	existing.Status = desired.Status

	rp, err := c.ProjectriffClientSet.RunV1alpha1().RequestProcessors(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(rp.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("RequestProcessor %q became ready after %v", requestprocessor.Name, duration)
	}

	return rp, err
}

func (c *Reconciler) reconcileBuilds(ctx context.Context, requestprocessor *runv1alpha1.RequestProcessor) ([]*corev1.ObjectReference, error) {
	logger := logging.FromContext(ctx)

	buildRefs := make([]*corev1.ObjectReference, len(requestprocessor.Spec))
	buildSet := sets.NewString()
	for i := range requestprocessor.Spec {
		if requestprocessor.Spec[i].Build == nil {
			continue
		}
		build, err := c.reconcileBuild(ctx, requestprocessor, i)
		if err != nil {
			return nil, err
		}
		buildRef := &corev1.ObjectReference{
			APIVersion: build.GetGroupVersionKind().GroupVersion().String(),
			Kind:       build.GetGroupVersionKind().Kind,
			Name:       build.GetObjectMeta().GetName(),
			UID:        build.GetObjectMeta().GetUID(),
		}
		buildSet.Insert(fmt.Sprintf("%s/%s", buildRef.Kind, buildRef.Name))
		buildRefs[i] = buildRef
	}

	// drop orphaned builds
	for _, buildRef := range requestprocessor.Status.Builds {
		if buildRef == nil || buildSet.Has(fmt.Sprintf("%s/%s", buildRef.Kind, buildRef.Name)) {
			continue
		}
		switch buildRef.Kind {
		case "Application":
			application, err := c.applicationLister.Applications(requestprocessor.Namespace).Get(buildRef.Name)
			if errors.IsNotFound(err) {
				// can't delete something that doesn't exist
				continue
			} else if err != nil {
				logger.Errorf("Failed to reconcile RequestProcessor: %q failed to Get Application: %q; %v", requestprocessor.Name, buildRef.Name, zap.Error(err))
				return nil, err
			} else if !metav1.IsControlledBy(application, requestprocessor) {
				// it's not ours, don't delete it
				continue
			}
			if err := c.ProjectriffClientSet.BuildV1alpha1().Applications(requestprocessor.Namespace).Delete(buildRef.Name, &metav1.DeleteOptions{
				Preconditions: &metav1.Preconditions{UID: &buildRef.UID},
			}); err != nil {
				logger.Errorf("Failed to reconcile RequestProcessor: %q failed to delete orphaned Application: %q; %v", requestprocessor.Name, application, zap.Error(err))
				return nil, err
			}
			c.Recorder.Eventf(requestprocessor, corev1.EventTypeNormal, "Deleted", "Deleted Application %q", buildRef.Name)

		case "Function":
			function, err := c.functionLister.Functions(requestprocessor.Namespace).Get(buildRef.Name)
			if errors.IsNotFound(err) {
				// can't delete something that doesn't exist
				continue
			} else if err != nil {
				logger.Errorf("Failed to reconcile RequestProcessor: %q failed to Get Function: %q; %v", requestprocessor.Name, buildRef.Name, zap.Error(err))
				return nil, err
			} else if !metav1.IsControlledBy(function, requestprocessor) {
				// it's not ours, don't delete it
				continue
			}
			if err := c.ProjectriffClientSet.BuildV1alpha1().Functions(requestprocessor.Namespace).Delete(buildRef.Name, &metav1.DeleteOptions{
				Preconditions: &metav1.Preconditions{UID: &buildRef.UID},
			}); err != nil {
				logger.Errorf("Failed to reconcile RequestProcessor: %q failed to delete orphaned Function: %q; %v", requestprocessor.Name, function, zap.Error(err))
				return nil, err
			}
			c.Recorder.Eventf(requestprocessor, corev1.EventTypeNormal, "Deleted", "Deleted Function %q", buildRef.Name)

		default:
			requestprocessor.Status.MarkBuildsFailed("Kind", fmt.Sprintf("Unknown build of kind %q", buildRef.Kind))
			return nil, fmt.Errorf("Unknown build of kind %q", buildRef.Kind)
		}
	}

	return buildRefs, nil
}

func (c *Reconciler) reconcileBuild(ctx context.Context, requestprocessor *runv1alpha1.RequestProcessor, i int) (kmeta.OwnerRefable, error) {
	logger := logging.FromContext(ctx)

	build := requestprocessor.Spec[i].Build
	switch {
	case build.Application != nil:
		applicationName := resourcenames.TagOrIndex(requestprocessor, i)
		application, err := c.applicationLister.Applications(requestprocessor.Namespace).Get(applicationName)
		if errors.IsNotFound(err) {
			application, err = c.createApplication(requestprocessor, i)
			if err != nil {
				logger.Errorf("Failed to create Application %q: %v", applicationName, err)
				c.Recorder.Eventf(requestprocessor, corev1.EventTypeWarning, "CreationFailed", "Failed to create Application %q: %v", applicationName, err)
				return nil, err
			}
			if application != nil {
				c.Recorder.Eventf(requestprocessor, corev1.EventTypeNormal, "Created", "Created Application %q", applicationName)
			}
		} else if err != nil {
			logger.Errorf("Failed to reconcile RequestProcessor: %q failed to Get Application: %q; %v", requestprocessor.Name, applicationName, zap.Error(err))
			return nil, err
		} else if !metav1.IsControlledBy(application, requestprocessor) {
			// Surface an error in the application's status,and return an error.
			requestprocessor.Status.MarkBuildNotOwned(application.Kind, application.Name)
			return nil, fmt.Errorf("RequestProcessor: %q does not own Application: %q", requestprocessor.Name, applicationName)
		} else if application, err = c.reconcileApplication(ctx, requestprocessor, i, application); err != nil {
			logger.Errorf("Failed to reconcile RequestProcessor: %q failed to reconcile Application: %q; %v", requestprocessor.Name, application.Name, zap.Error(err))
			return nil, err
		}
		return application, nil

	case build.ApplicationRef != "":
		application, err := c.applicationLister.Applications(requestprocessor.Namespace).Get(build.ApplicationRef)
		if err != nil {
			return nil, err
		}
		gvk := buildv1alpha1.SchemeGroupVersion.WithKind("Application")
		if err := c.tracker.Track(objectRef(application, gvk), requestprocessor); err != nil {
			return nil, err
		}
		return application, nil

	case build.Function != nil:
		functionName := resourcenames.TagOrIndex(requestprocessor, i)
		function, err := c.functionLister.Functions(requestprocessor.Namespace).Get(functionName)
		if errors.IsNotFound(err) {
			function, err = c.createFunction(requestprocessor, i)
			if err != nil {
				logger.Errorf("Failed to create Function %q: %v", functionName, err)
				c.Recorder.Eventf(requestprocessor, corev1.EventTypeWarning, "CreationFailed", "Failed to create Function %q: %v", functionName, err)
				return nil, err
			}
			if function != nil {
				c.Recorder.Eventf(requestprocessor, corev1.EventTypeNormal, "Created", "Created Function %q", functionName)
			}
		} else if err != nil {
			logger.Errorf("Failed to reconcile RequestProcessor: %q failed to Get Function: %q; %v", requestprocessor.Name, functionName, zap.Error(err))
			return nil, err
		} else if !metav1.IsControlledBy(function, requestprocessor) {
			// Surface an error in the function's status,and return an error.
			requestprocessor.Status.MarkBuildNotOwned(function.Kind, function.Name)
			return nil, fmt.Errorf("RequestProcessor: %q does not own Function: %q", requestprocessor.Name, functionName)
		} else if function, err = c.reconcileFunction(ctx, requestprocessor, i, function); err != nil {
			logger.Errorf("Failed to reconcile RequestProcessor: %q failed to reconcile Function: %q; %v", requestprocessor.Name, function.Name, zap.Error(err))
			return nil, err
		}
		return function, nil

	case build.FunctionRef != "":
		function, err := c.functionLister.Functions(requestprocessor.Namespace).Get(build.FunctionRef)
		if err != nil {
			return nil, err
		}
		gvk := buildv1alpha1.SchemeGroupVersion.WithKind("Function")
		if err := c.tracker.Track(objectRef(function, gvk), requestprocessor); err != nil {
			return nil, err
		}
		return function, nil

	}
	// should never get here
	panic(fmt.Sprintf("invalid build %+v", build))
}

func (c *Reconciler) reconcileApplication(ctx context.Context, requestprocessor *runv1alpha1.RequestProcessor, i int, application *buildv1alpha1.Application) (*buildv1alpha1.Application, error) {
	logger := logging.FromContext(ctx)
	desiredApplication, err := resources.MakeApplication(requestprocessor, i)
	if err != nil {
		return nil, err
	}

	if applicationSemanticEquals(desiredApplication, application) {
		// No differences to reconcile.
		return application, nil
	}
	diff, err := kmp.SafeDiff(desiredApplication.Spec, application.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff Application: %v", err)
	}
	logger.Infof("Reconciling application diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := application.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredApplication.Spec
	existing.ObjectMeta.Labels = desiredApplication.ObjectMeta.Labels
	return c.ProjectriffClientSet.BuildV1alpha1().Applications(requestprocessor.Namespace).Update(existing)
}

func (c *Reconciler) createApplication(requestprocessor *runv1alpha1.RequestProcessor, i int) (*buildv1alpha1.Application, error) {
	application, err := resources.MakeApplication(requestprocessor, i)
	if err != nil {
		return nil, err
	}
	if application == nil {
		// nothing to create
		return application, nil
	}
	return c.ProjectriffClientSet.BuildV1alpha1().Applications(requestprocessor.Namespace).Create(application)
}

func applicationSemanticEquals(desiredApplication, application *buildv1alpha1.Application) bool {
	return equality.Semantic.DeepEqual(desiredApplication.Spec, application.Spec) &&
		equality.Semantic.DeepEqual(desiredApplication.ObjectMeta.Labels, application.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileFunction(ctx context.Context, requestprocessor *runv1alpha1.RequestProcessor, i int, function *buildv1alpha1.Function) (*buildv1alpha1.Function, error) {
	logger := logging.FromContext(ctx)
	desiredFunction, err := resources.MakeFunction(requestprocessor, i)
	if err != nil {
		return nil, err
	}

	if functionSemanticEquals(desiredFunction, function) {
		// No differences to reconcile.
		return function, nil
	}
	diff, err := kmp.SafeDiff(desiredFunction.Spec, function.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff Function: %v", err)
	}
	logger.Infof("Reconciling function diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := function.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredFunction.Spec
	existing.ObjectMeta.Labels = desiredFunction.ObjectMeta.Labels
	return c.ProjectriffClientSet.BuildV1alpha1().Functions(requestprocessor.Namespace).Update(existing)
}

func (c *Reconciler) createFunction(requestprocessor *runv1alpha1.RequestProcessor, i int) (*buildv1alpha1.Function, error) {
	function, err := resources.MakeFunction(requestprocessor, i)
	if err != nil {
		return nil, err
	}
	if function == nil {
		// nothing to create
		return function, nil
	}
	return c.ProjectriffClientSet.BuildV1alpha1().Functions(requestprocessor.Namespace).Create(function)
}

func functionSemanticEquals(desiredFunction, function *buildv1alpha1.Function) bool {
	return equality.Semantic.DeepEqual(desiredFunction.Spec, function.Spec) &&
		equality.Semantic.DeepEqual(desiredFunction.ObjectMeta.Labels, function.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileConfigurations(ctx context.Context, requestprocessor *runv1alpha1.RequestProcessor) ([]knservingv1alpha1.Configuration, error) {
	logger := logging.FromContext(ctx)

	configurationNames := make([]string, len(requestprocessor.Spec))
	for i := range requestprocessor.Spec {
		if requestprocessor.Spec[i].Tag != "" {
			configurationNames[i] = fmt.Sprintf("%s-%s", resourcenames.Configuration(requestprocessor), requestprocessor.Spec[i].Tag)
		} else {
			configurationNames[i] = fmt.Sprintf("%s-%d", resourcenames.Configuration(requestprocessor), i)
		}
	}
	if err := c.resolveImages(ctx, requestprocessor); err != nil {
		c.Recorder.Eventf(requestprocessor, corev1.EventTypeWarning, "ResolutionFailed", "Failed to resolve image: %v", err)
		return nil, fmt.Errorf("unable to resolve image: %v", err)
	}
	configurations := make([]knservingv1alpha1.Configuration, len(requestprocessor.Spec))
	for i, configurationName := range configurationNames {
		configuration, err := c.knconfigurationLister.Configurations(requestprocessor.Namespace).Get(configurationName)
		if errors.IsNotFound(err) {
			configuration, err = c.createConfiguration(requestprocessor, i)
			if err != nil {
				logger.Errorf("Failed to create Configuration %q: %v", configurationName, err)
				c.Recorder.Eventf(requestprocessor, corev1.EventTypeWarning, "CreationFailed", "Failed to create Configuration %q: %v", configurationName, err)
				return nil, err
			}
			if configuration != nil {
				c.Recorder.Eventf(requestprocessor, corev1.EventTypeNormal, "Created", "Created Configuration %q", configurationName)
			}
		} else if err != nil {
			logger.Errorf("Failed to reconcile RequestProcessor: %q failed to Get Configuration: %q; %v", requestprocessor.Name, configurationName, zap.Error(err))
			return nil, err
		} else if !metav1.IsControlledBy(configuration, requestprocessor) {
			// Surface an error in the requestprocessor's status,and return an error.
			requestprocessor.Status.MarkConfigurationNotOwned(configurationName)
			return nil, fmt.Errorf("RequestProcessor: %q does not own Configuration: %q", requestprocessor.Name, configurationName)
		} else {
			configuration, err = c.reconcileConfiguration(ctx, requestprocessor, configuration, i)
			if err != nil {
				logger.Errorf("Failed to reconcile RequestProcessor: %q failed to reconcile Configuration: %q; %v", requestprocessor.Name, configuration, zap.Error(err))
				return nil, err
			}
			if configuration == nil {
				c.Recorder.Eventf(requestprocessor, corev1.EventTypeNormal, "Deleted", "Deleted Configuration %q", configurationName)
			}
		}
		configurations[i] = *configuration
	}
	// drop orphaned configurations
	configurationNameSet := sets.NewString(configurationNames...)
	for _, configurationName := range requestprocessor.Status.ConfigurationNames {
		if configurationNameSet.Has(configurationName) {
			continue
		}
		configuration, err := c.knconfigurationLister.Configurations(requestprocessor.Namespace).Get(configurationName)
		if errors.IsNotFound(err) {
			// can't delete something that doesn't exist
			continue
		} else if err != nil {
			logger.Errorf("Failed to reconcile RequestProcessor: %q failed to Get Configuration: %q; %v", requestprocessor.Name, configurationName, zap.Error(err))
			return nil, err
		} else if !metav1.IsControlledBy(configuration, requestprocessor) {
			// it's not ours, don't delete it
			continue
		}
		if err := c.KnServingClientSet.ServingV1alpha1().Configurations(requestprocessor.Namespace).Delete(configuration.Name, &metav1.DeleteOptions{
			Preconditions: &metav1.Preconditions{UID: &configuration.UID},
		}); err != nil {
			logger.Errorf("Failed to reconcile RequestProcessor: %q failed to delete orphaned Configuration: %q; %v", requestprocessor.Name, configuration, zap.Error(err))
			return nil, err
		}
		c.Recorder.Eventf(requestprocessor, corev1.EventTypeNormal, "Deleted", "Deleted Configuration %q", configurationName)
	}

	return configurations, nil
}

func (c *Reconciler) resolveImages(ctx context.Context, rp *runv1alpha1.RequestProcessor) error {
	for i := range rp.Spec {
		ref := rp.Status.Builds[i]
		if ref == nil {
			continue
		}

		switch {
		case ref.Kind == "Application":
			build, err := c.applicationLister.Applications(rp.Namespace).Get(ref.Name)
			if err != nil {
				return err
			}
			rp.Spec[i].Containers[0].Image = build.Status.LatestImage
		case ref.Kind == "Function":
			build, err := c.functionLister.Functions(rp.Namespace).Get(ref.Name)
			if err != nil {
				return err
			}
			rp.Spec[i].Containers[0].Image = build.Status.LatestImage
		}
	}

	return nil
}

func (c *Reconciler) reconcileConfiguration(ctx context.Context, requestprocessor *runv1alpha1.RequestProcessor, configuration *knservingv1alpha1.Configuration, i int) (*knservingv1alpha1.Configuration, error) {
	logger := logging.FromContext(ctx)

	desiredConfiguration, err := resources.MakeConfiguration(requestprocessor, i)
	if err != nil {
		return nil, err
	}

	if desiredConfiguration == nil {
		// drop an existing configuration
		return nil, c.KnServingClientSet.ServingV1alpha1().Configurations(requestprocessor.Namespace).Delete(configuration.Name, &metav1.DeleteOptions{
			Preconditions: &metav1.Preconditions{UID: &configuration.UID},
		})
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
	return c.KnServingClientSet.ServingV1alpha1().Configurations(requestprocessor.Namespace).Update(existing)
}

func (c *Reconciler) createConfiguration(requestprocessor *runv1alpha1.RequestProcessor, i int) (*knservingv1alpha1.Configuration, error) {
	configuration, err := resources.MakeConfiguration(requestprocessor, i)
	if err != nil {
		return nil, err
	}
	if configuration == nil {
		// nothing to create
		return configuration, nil
	}
	return c.KnServingClientSet.ServingV1alpha1().Configurations(requestprocessor.Namespace).Create(configuration)
}

func configurationSemanticEquals(desiredConfiguration, configuration *knservingv1alpha1.Configuration) bool {
	return equality.Semantic.DeepEqual(desiredConfiguration.Spec, configuration.Spec) &&
		equality.Semantic.DeepEqual(desiredConfiguration.ObjectMeta.Labels, configuration.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileRoute(ctx context.Context, requestprocessor *runv1alpha1.RequestProcessor, route *knservingv1alpha1.Route) (*knservingv1alpha1.Route, error) {
	logger := logging.FromContext(ctx)
	desiredRoute, err := resources.MakeRoute(requestprocessor)
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
	return c.KnServingClientSet.ServingV1alpha1().Routes(requestprocessor.Namespace).Update(existing)
}

func (c *Reconciler) createRoute(requestprocessor *runv1alpha1.RequestProcessor) (*knservingv1alpha1.Route, error) {
	route, err := resources.MakeRoute(requestprocessor)
	if err != nil {
		return nil, err
	}
	return c.KnServingClientSet.ServingV1alpha1().Routes(requestprocessor.Namespace).Create(route)
}

func routeSemanticEquals(desiredRoute, route *knservingv1alpha1.Route) bool {
	return equality.Semantic.DeepEqual(desiredRoute.Spec, route.Spec) &&
		equality.Semantic.DeepEqual(desiredRoute.ObjectMeta.Labels, route.ObjectMeta.Labels)
}

/////////////////////////////////////////
// Misc helpers.
/////////////////////////////////////////

type accessor interface {
	GroupVersionKind() schema.GroupVersionKind
	GetNamespace() string
	GetName() string
}

func objectRef(a accessor, gvk schema.GroupVersionKind) corev1.ObjectReference {
	// We can't always rely on the TypeMeta being populated.
	// See: https://github.com/knative/serving/issues/2372
	// Also: https://github.com/kubernetes/apiextensions-apiserver/issues/29
	// gvk := a.GroupVersionKind()
	apiVersion, kind := gvk.ToAPIVersionAndKind()
	return corev1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       kind,
		Namespace:  a.GetNamespace(),
		Name:       a.GetName(),
	}
}
