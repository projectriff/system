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

package application

import (
	"context"
	"fmt"
	"reflect"
	"time"

	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	buildinformers "github.com/knative/build/pkg/client/informers/externalversions/build/v1alpha1"
	buildlisters "github.com/knative/build/pkg/client/listers/build/v1alpha1"
	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/kmp"
	"github.com/knative/pkg/logging"
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	servinginformers "github.com/knative/serving/pkg/client/informers/externalversions/serving/v1alpha1"
	servinglisters "github.com/knative/serving/pkg/client/listers/serving/v1alpha1"
	projectriffv1alpha1 "github.com/projectriff/system/pkg/apis/projectriff/v1alpha1"
	projectriffinformers "github.com/projectriff/system/pkg/client/informers/externalversions/projectriff/v1alpha1"
	projectrifflisters "github.com/projectriff/system/pkg/client/listers/projectriff/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/application/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/application/resources/names"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	// ReconcilerName is the name of the reconciler
	ReconcilerName      = "Applications"
	controllerAgentName = "application-controller"
)

// Reconciler implements controller.Reconciler for Application resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	applicationLister   projectrifflisters.ApplicationLister
	pvcLister           corelisters.PersistentVolumeClaimLister
	buildLister         buildlisters.BuildLister
	configurationLister servinglisters.ConfigurationLister
	routeLister         servinglisters.RouteLister
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	applicationInformer projectriffinformers.ApplicationInformer,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
	buildInformer buildinformers.BuildInformer,
	configurationInformer servinginformers.ConfigurationInformer,
	routeInformer servinginformers.RouteInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:                reconciler.NewBase(opt, controllerAgentName),
		applicationLister:   applicationInformer.Lister(),
		pvcLister:           pvcInformer.Lister(),
		buildLister:         buildInformer.Lister(),
		configurationLister: configurationInformer.Lister(),
		routeLister:         routeInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	applicationInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
		DeleteFunc: impl.Enqueue,
	})

	pvcInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(projectriffv1alpha1.SchemeGroupVersion.WithKind("Application")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})
	buildInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(projectriffv1alpha1.SchemeGroupVersion.WithKind("Application")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})
	configurationInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(projectriffv1alpha1.SchemeGroupVersion.WithKind("Application")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})
	routeInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(projectriffv1alpha1.SchemeGroupVersion.WithKind("Application")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Application resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the Application resource with this namespace/name
	original, err := c.applicationLister.Applications(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("application %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	application := original.DeepCopy()

	// Reconcile this copy of the application and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, application)

	if equality.Semantic.DeepEqual(original.Status, application.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(application); uErr != nil {
		logger.Warn("Failed to update application status", zap.Error(uErr))
		c.Recorder.Eventf(application, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for Application %q: %v", application.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(application, corev1.EventTypeNormal, "Updated", "Updated Application %q", application.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, application *projectriffv1alpha1.Application) error {
	logger := logging.FromContext(ctx)
	if application.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	application.SetDefaults(context.Background())

	application.Status.InitializeConditions()

	buildCacheName := resourcenames.BuildCache(application)
	buildCache, err := c.pvcLister.PersistentVolumeClaims(application.Namespace).Get(buildCacheName)
	if errors.IsNotFound(err) {
		buildCache, err = c.createBuildCache(application)
		if err != nil {
			logger.Errorf("Failed to create PersistentVolumeClaim %q: %v", buildCacheName, err)
			c.Recorder.Eventf(application, corev1.EventTypeWarning, "CreationFailed", "Failed to create PersistentVolumeClaim %q: %v", buildCacheName, err)
			return err
		}
		if buildCache != nil {
			c.Recorder.Eventf(application, corev1.EventTypeNormal, "Created", "Created PersistentVolumeClaim %q", buildCacheName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Application: %q failed to Get PersistentVolumeClaim: %q; %v", application.Name, buildCacheName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(buildCache, application) {
		// Surface an error in the application's status,and return an error.
		application.Status.MarkBuildCacheNotOwned(buildCacheName)
		return fmt.Errorf("Application: %q does not own PersistentVolumeClaim: %q", application.Name, buildCacheName)
	} else if buildCache, err = c.reconcileBuildCache(ctx, application, buildCache); err != nil {
		logger.Errorf("Failed to reconcile Application: %q failed to reconcile PersistentVolumeClaim: %q; %v", application.Name, buildCache, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying PersistentVolumeClaim.
	application.Status.PropagateBuildCacheStatus(buildCache)

	buildName := resourcenames.Build(application)
	build, err := c.buildLister.Builds(application.Namespace).Get(buildName)
	if errors.IsNotFound(err) {
		build, err = c.createBuild(application)
		if err != nil {
			logger.Errorf("Failed to create Build %q: %v", buildName, err)
			c.Recorder.Eventf(application, corev1.EventTypeWarning, "CreationFailed", "Failed to create Build %q: %v", buildName, err)
			return err
		}
		if build != nil {
			c.Recorder.Eventf(application, corev1.EventTypeNormal, "Created", "Created Build %q", buildName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Application: %q failed to Get Build: %q; %v", application.Name, buildName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(build, application) {
		// Surface an error in the application's status,and return an error.
		application.Status.MarkBuildNotOwned(buildName)
		return fmt.Errorf("Application: %q does not own Build: %q", application.Name, buildName)
	} else if build, err = c.reconcileBuild(ctx, application, build); err != nil {
		logger.Errorf("Failed to reconcile Application: %q failed to reconcile Build: %q; %v", application.Name, build, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Build.
	application.Status.PropagateBuildStatus(build)

	configurationName := resourcenames.Configuration(application)
	configuration, err := c.configurationLister.Configurations(application.Namespace).Get(configurationName)
	if errors.IsNotFound(err) {
		// wait for the build to succeed before deploying image
		if application.Status.GetCondition(projectriffv1alpha1.ApplicationConditionBuildSucceeded).IsTrue() {
			configuration, err = c.createConfiguration(application)
			if err != nil {
				logger.Errorf("Failed to create Configuration %q: %v", configurationName, err)
				c.Recorder.Eventf(application, corev1.EventTypeWarning, "CreationFailed", "Failed to create Configuration %q: %v", configurationName, err)
				return err
			}
			c.Recorder.Eventf(application, corev1.EventTypeNormal, "Created", "Created Configuration %q", configurationName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Application: %q failed to Get Configuration: %q; %v", application.Name, configurationName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(configuration, application) {
		// Surface an error in the application's status,and return an error.
		application.Status.MarkConfigurationNotOwned(configurationName)
		return fmt.Errorf("Application: %q does not own Configuration: %q", application.Name, configurationName)
	} else if configuration, err = c.reconcileConfiguration(ctx, application, buildCache, configuration); err != nil {
		logger.Errorf("Failed to reconcile Application: %q failed to reconcile Configuration: %q; %v", application.Name, configurationName, zap.Error(err))
		return err
	}

	if configuration != nil {
		// Update our Status based on the state of our underlying Configuration.
		application.Status.PropagateConfigurationStatus(&configuration.Status)
	}

	routeName := resourcenames.Route(application)
	route, err := c.routeLister.Routes(application.Namespace).Get(routeName)
	if errors.IsNotFound(err) {
		route, err = c.createRoute(application)
		if err != nil {
			logger.Errorf("Failed to create Route %q: %v", routeName, err)
			c.Recorder.Eventf(application, corev1.EventTypeWarning, "CreationFailed", "Failed to create Route %q: %v", routeName, err)
			return err
		}
		c.Recorder.Eventf(application, corev1.EventTypeNormal, "Created", "Created Route %q", routeName)
	} else if err != nil {
		logger.Errorf("Failed to reconcile Application: %q failed to Get Route: %q; %v", application.Name, routeName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(route, application) {
		// Surface an error in the application's status,and return an error.
		application.Status.MarkRouteNotOwned(routeName)
		return fmt.Errorf("Application: %q does not own Route: %q", application.Name, routeName)
	} else if route, err = c.reconcileRoute(ctx, application, route); err != nil {
		logger.Errorf("Failed to reconcile Application: %q failed to reconcile Route: %q; %v", application.Name, routeName, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Route.
	application.Status.PropagateRouteStatus(&route.Status)

	application.Status.ObservedGeneration = application.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *projectriffv1alpha1.Application) (*projectriffv1alpha1.Application, error) {
	application, err := c.applicationLister.Applications(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(application.Status, desired.Status) {
		return application, nil
	}
	becomesReady := desired.Status.IsReady() && !application.Status.IsReady()
	// Don't modify the informers copy.
	existing := application.DeepCopy()
	existing.Status = desired.Status

	svc, err := c.ProjectriffClientSet.ProjectriffV1alpha1().Applications(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(svc.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("Application %q became ready after %v", application.Name, duration)
	}

	return svc, err
}

func (c *Reconciler) reconcileBuildCache(ctx context.Context, application *projectriffv1alpha1.Application, buildCache *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	logger := logging.FromContext(ctx)
	desiredBuildCache, err := resources.MakeBuildCache(application)
	if err != nil {
		return nil, err
	}

	if buildCache != nil && desiredBuildCache == nil {
		// TODO drop an existing buildCache
		// return nil, c.KubeClientSet.CoreV1().PersistentVolumeClaims(application.Namespace).Delete(buildCache.Name)
		return nil, nil
	}

	if buildCacheSemanticEquals(desiredBuildCache, buildCache) {
		// No differences to reconcile.
		return buildCache, nil
	}
	diff, err := kmp.SafeDiff(desiredBuildCache.Spec, buildCache.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff PersistentVolumeClaim: %v", err)
	}
	logger.Infof("Reconciling build cache diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := buildCache.DeepCopy()
	// Preserve the rest of the object (e.g. PersistentVolumeClaimSpec except for resources).
	existing.Spec.Resources = desiredBuildCache.Spec.Resources
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.ObjectMeta.Labels = desiredBuildCache.ObjectMeta.Labels
	return c.KubeClientSet.CoreV1().PersistentVolumeClaims(application.Namespace).Update(existing)
}

func (c *Reconciler) createBuildCache(application *projectriffv1alpha1.Application) (*corev1.PersistentVolumeClaim, error) {
	buildCache, err := resources.MakeBuildCache(application)
	if err != nil {
		return nil, err
	}
	if buildCache == nil {
		// nothing to create
		return buildCache, nil
	}
	return c.KubeClientSet.CoreV1().PersistentVolumeClaims(application.Namespace).Create(buildCache)
}

func buildCacheSemanticEquals(desiredBuildCache, buildCache *corev1.PersistentVolumeClaim) bool {
	return equality.Semantic.DeepEqual(desiredBuildCache.Spec.Resources, buildCache.Spec.Resources) &&
		equality.Semantic.DeepEqual(desiredBuildCache.ObjectMeta.Labels, buildCache.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileBuild(ctx context.Context, application *projectriffv1alpha1.Application, build *buildv1alpha1.Build) (*buildv1alpha1.Build, error) {
	logger := logging.FromContext(ctx)
	desiredBuild, err := resources.MakeBuild(application)
	if err != nil {
		return nil, err
	}

	if build != nil && desiredBuild == nil {
		// TODO drop a existing build
		// return nil, c.buildClientSet.BuildV1alpha1().Builds(application.Namespace).Delete(build.Name)
		return nil, nil
	}

	if buildSemanticEquals(desiredBuild, build) {
		// No differences to reconcile.
		return build, nil
	}
	diff, err := kmp.SafeDiff(desiredBuild.Spec, build.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff Build: %v", err)
	}
	logger.Infof("Reconciling build diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := build.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredBuild.Spec
	existing.ObjectMeta.Labels = desiredBuild.ObjectMeta.Labels
	return c.BuildClientSet.BuildV1alpha1().Builds(application.Namespace).Update(existing)
}

func (c *Reconciler) createBuild(application *projectriffv1alpha1.Application) (*buildv1alpha1.Build, error) {
	build, err := resources.MakeBuild(application)
	if err != nil {
		return nil, err
	}
	if build == nil {
		// nothing to create
		return build, nil
	}
	return c.BuildClientSet.BuildV1alpha1().Builds(application.Namespace).Create(build)
}

func buildSemanticEquals(desiredBuild, build *buildv1alpha1.Build) bool {
	return equality.Semantic.DeepEqual(desiredBuild.Spec, build.Spec) &&
		equality.Semantic.DeepEqual(desiredBuild.ObjectMeta.Labels, build.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileConfiguration(ctx context.Context, application *projectriffv1alpha1.Application, buildCache *corev1.PersistentVolumeClaim, configuration *servingv1alpha1.Configuration) (*servingv1alpha1.Configuration, error) {
	logger := logging.FromContext(ctx)
	desiredConfiguration, err := resources.MakeConfiguration(application)
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
	return c.ServingClientSet.ServingV1alpha1().Configurations(application.Namespace).Update(existing)
}

func (c *Reconciler) createConfiguration(application *projectriffv1alpha1.Application) (*servingv1alpha1.Configuration, error) {
	configuration, err := resources.MakeConfiguration(application)
	if err != nil {
		return nil, err
	}
	return c.ServingClientSet.ServingV1alpha1().Configurations(application.Namespace).Create(configuration)
}

func configurationSemanticEquals(desiredConfiguration, configuration *servingv1alpha1.Configuration) bool {
	return equality.Semantic.DeepEqual(desiredConfiguration.Spec, configuration.Spec) &&
		equality.Semantic.DeepEqual(desiredConfiguration.ObjectMeta.Labels, configuration.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileRoute(ctx context.Context, application *projectriffv1alpha1.Application, route *servingv1alpha1.Route) (*servingv1alpha1.Route, error) {
	logger := logging.FromContext(ctx)
	desiredRoute, err := resources.MakeRoute(application)
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
	return c.ServingClientSet.ServingV1alpha1().Routes(application.Namespace).Update(existing)
}

func (c *Reconciler) createRoute(application *projectriffv1alpha1.Application) (*servingv1alpha1.Route, error) {
	route, err := resources.MakeRoute(application)
	if err != nil {
		return nil, err
	}
	return c.ServingClientSet.ServingV1alpha1().Routes(application.Namespace).Create(route)
}

func routeSemanticEquals(desiredRoute, route *servingv1alpha1.Route) bool {
	return equality.Semantic.DeepEqual(desiredRoute.Spec, route.Spec) &&
		equality.Semantic.DeepEqual(desiredRoute.ObjectMeta.Labels, route.ObjectMeta.Labels)
}
