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
	applicationLister projectrifflisters.ApplicationLister
	serviceLister     servinglisters.ServiceLister
	pvcLister         corelisters.PersistentVolumeClaimLister
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	applicationInformer projectriffinformers.ApplicationInformer,
	serviceInformer servinginformers.ServiceInformer,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:              reconciler.NewBase(opt, controllerAgentName),
		applicationLister: applicationInformer.Lister(),
		serviceLister:     serviceInformer.Lister(),
		pvcLister:         pvcInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	applicationInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
		DeleteFunc: impl.Enqueue,
	})

	serviceInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
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

	serviceName := resourcenames.Service(application)
	service, err := c.serviceLister.Services(application.Namespace).Get(serviceName)
	if errors.IsNotFound(err) {
		service, err = c.createService(application)
		if err != nil {
			logger.Errorf("Failed to create Service %q: %v", serviceName, err)
			c.Recorder.Eventf(application, corev1.EventTypeWarning, "CreationFailed", "Failed to create Service %q: %v", serviceName, err)
			return err
		}
		c.Recorder.Eventf(application, corev1.EventTypeNormal, "Created", "Created Service %q", serviceName)
	} else if err != nil {
		logger.Errorf("Failed to reconcile Application: %q failed to Get Service: %q; %v", application.Name, serviceName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(service, application) {
		// Surface an error in the application's status,and return an error.
		application.Status.MarkServiceNotOwned(serviceName)
		return fmt.Errorf("Application: %q does not own Service: %q", application.Name, serviceName)
	} else if service, err = c.reconcileService(ctx, application, buildCache, service); err != nil {
		logger.Errorf("Failed to reconcile Application: %q failed to reconcile Service: %q; %v", application.Name, serviceName, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Service.
	application.Status.PropagateServiceStatus(&service.Status)

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

func (c *Reconciler) reconcileService(ctx context.Context, application *projectriffv1alpha1.Application, buildCache *corev1.PersistentVolumeClaim, service *servingv1alpha1.Service) (*servingv1alpha1.Service, error) {
	logger := logging.FromContext(ctx)
	desiredService, err := resources.MakeService(application)
	if err != nil {
		return nil, err
	}

	if serviceSemanticEquals(desiredService, service) {
		// No differences to reconcile.
		return service, nil
	}
	diff, err := kmp.SafeDiff(desiredService.Spec, service.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff Service: %v", err)
	}
	logger.Infof("Reconciling service diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := service.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredService.Spec
	existing.ObjectMeta.Labels = desiredService.ObjectMeta.Labels
	return c.ServingClientSet.ServingV1alpha1().Services(application.Namespace).Update(existing)
}

func (c *Reconciler) createService(application *projectriffv1alpha1.Application) (*servingv1alpha1.Service, error) {
	cfg, err := resources.MakeService(application)
	if err != nil {
		return nil, err
	}
	return c.ServingClientSet.ServingV1alpha1().Services(application.Namespace).Create(cfg)
}

func serviceSemanticEquals(desiredService, service *servingv1alpha1.Service) bool {
	return equality.Semantic.DeepEqual(desiredService.Spec, service.Spec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels)
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
	existing := desiredBuildCache.DeepCopy()
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
