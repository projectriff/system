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

package corehandler

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/kmp"
	"github.com/knative/pkg/logging"
	"github.com/knative/pkg/tracker"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	corev1alpha1 "github.com/projectriff/system/pkg/apis/core/v1alpha1"
	buildinformers "github.com/projectriff/system/pkg/client/informers/externalversions/build/v1alpha1"
	coreruntimeinformers "github.com/projectriff/system/pkg/client/informers/externalversions/core/v1alpha1"
	buildlisters "github.com/projectriff/system/pkg/client/listers/build/v1alpha1"
	coreruntimelisters "github.com/projectriff/system/pkg/client/listers/core/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/corehandler/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/corehandler/resources/names"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	// ReconcilerName is the name of the reconciler
	ReconcilerName      = "Handlers"
	controllerAgentName = "handler-controller"
)

// Reconciler implements controller.Reconciler for Handler resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	handlerLister     coreruntimelisters.HandlerLister
	deploymentLister  appslisters.DeploymentLister
	serviceLister     corelisters.ServiceLister
	applicationLister buildlisters.ApplicationLister
	containerLister   buildlisters.ContainerLister
	functionLister    buildlisters.FunctionLister

	tracker tracker.Interface
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	handlerInformer coreruntimeinformers.HandlerInformer,
	deploymentInformer appsinformers.DeploymentInformer,
	serviceInformer coreinformers.ServiceInformer,
	applicationInformer buildinformers.ApplicationInformer,
	containerInformer buildinformers.ContainerInformer,
	functionInformer buildinformers.FunctionInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:              reconciler.NewBase(opt, controllerAgentName),
		handlerLister:     handlerInformer.Lister(),
		deploymentLister:  deploymentInformer.Lister(),
		serviceLister:     serviceInformer.Lister(),
		applicationLister: applicationInformer.Lister(),
		containerLister:   containerInformer.Lister(),
		functionLister:    functionInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName)

	c.Logger.Info("Setting up event handlers")
	handlerInformer.Informer().AddEventHandler(reconciler.Handler(impl.Enqueue))

	// controlled resources
	deploymentInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(corev1alpha1.SchemeGroupVersion.WithKind("Handler")),
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})
	serviceInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(corev1alpha1.SchemeGroupVersion.WithKind("Handler")),
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
// converge the two. It then updates the Status block of the Handler resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the Handler resource with this namespace/name
	original, err := c.handlerLister.Handlers(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("handler %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	handler := original.DeepCopy()

	// Reconcile this copy of the handler and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, handler)

	if equality.Semantic.DeepEqual(original.Status, handler.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(handler); uErr != nil {
		logger.Warn("Failed to update handler status", zap.Error(uErr))
		c.Recorder.Eventf(handler, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for Handler %q: %v", handler.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(handler, corev1.EventTypeNormal, "Updated", "Updated Handler %q", handler.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, handler *corev1alpha1.Handler) error {
	logger := logging.FromContext(ctx)
	if handler.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	handler.SetDefaults(context.Background())

	handler.Status.InitializeConditions()

	if err := c.reconcileBuild(handler); err != nil {
		return err
	}

	deploymentName := resourcenames.Deployment(handler)
	deployment, err := c.deploymentLister.Deployments(handler.Namespace).Get(deploymentName)
	if errors.IsNotFound(err) {
		deployment, err = c.createDeployment(handler)
		if err != nil {
			logger.Errorf("Failed to create Deployment %q: %v", deploymentName, err)
			c.Recorder.Eventf(handler, corev1.EventTypeWarning, "CreationFailed", "Failed to create Deployment %q: %v", deploymentName, err)
			return err
		}
		if deployment != nil {
			c.Recorder.Eventf(handler, corev1.EventTypeNormal, "Created", "Created Deployment %q", deploymentName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Handler: %q failed to Get Deployment: %q; %v", handler.Name, deploymentName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(deployment, handler) {
		// Surface an error in the handler's status,and return an error.
		handler.Status.MarkDeploymentNotOwned(deploymentName)
		return fmt.Errorf("Handler: %q does not own Deployment: %q", handler.Name, deploymentName)
	} else {
		deployment, err = c.reconcileDeployment(ctx, handler, deployment)
		if err != nil {
			logger.Errorf("Failed to reconcile Handler: %q failed to reconcile Deployment: %q; %v", handler.Name, deployment, zap.Error(err))
			return err
		}
	}

	// Update our Status based on the state of our underlying Deployment.
	handler.Status.DeploymentName = deployment.Name
	handler.Status.PropagateDeploymentStatus(&deployment.Status)

	serviceName := resourcenames.Service(handler)
	service, err := c.serviceLister.Services(handler.Namespace).Get(serviceName)
	if errors.IsNotFound(err) {
		service, err = c.createService(handler)
		if err != nil {
			logger.Errorf("Failed to create Service %q: %v", serviceName, err)
			c.Recorder.Eventf(handler, corev1.EventTypeWarning, "CreationFailed", "Failed to create Service %q: %v", serviceName, err)
			return err
		}
		if service != nil {
			c.Recorder.Eventf(handler, corev1.EventTypeNormal, "Created", "Created Service %q", serviceName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Handler: %q failed to Get Service: %q; %v", handler.Name, serviceName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(service, handler) {
		// Surface an error in the handler's status,and return an error.
		handler.Status.MarkServiceNotOwned(serviceName)
		return fmt.Errorf("Handler: %q does not own Service: %q", handler.Name, serviceName)
	} else if service, err = c.reconcileService(ctx, handler, service); err != nil {
		logger.Errorf("Failed to reconcile Handler: %q failed to reconcile Service: %q; %v", handler.Name, service, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Service.
	handler.Status.ServiceName = service.Name
	handler.Status.PropagateServiceStatus(&service.Status)

	handler.Status.ObservedGeneration = handler.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *corev1alpha1.Handler) (*corev1alpha1.Handler, error) {
	handler, err := c.handlerLister.Handlers(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(handler.Status, desired.Status) {
		return handler, nil
	}
	becomesReady := desired.Status.IsReady() && !handler.Status.IsReady()
	// Don't modify the informers copy.
	existing := handler.DeepCopy()
	existing.Status = desired.Status

	h, err := c.ProjectriffClientSet.CoreV1alpha1().Handlers(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(h.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("Handler %q became ready after %v", handler.Name, duration)
	}

	return h, err
}

func (c *Reconciler) reconcileBuild(handler *corev1alpha1.Handler) error {
	build := handler.Spec.Build
	if build == nil {
		return nil
	}

	switch {
	case build.ApplicationRef != "":
		application, err := c.applicationLister.Applications(handler.Namespace).Get(build.ApplicationRef)
		if err != nil {
			return err
		}
		if application.Status.LatestImage == "" {
			return fmt.Errorf("application %q does not have a ready image", build.ApplicationRef)
		}
		handler.Spec.Template.Containers[0].Image = application.Status.LatestImage

		// track application for new images
		gvk := buildv1alpha1.SchemeGroupVersion.WithKind("Application")
		if err := c.tracker.Track(reconciler.MakeObjectRef(application, gvk), handler); err != nil {
			return err
		}
		return nil

	case build.ContainerRef != "":
		container, err := c.containerLister.Containers(handler.Namespace).Get(build.ContainerRef)
		if err != nil {
			return err
		}
		if container.Status.LatestImage == "" {
			return fmt.Errorf("container %q does not have a ready image", build.ContainerRef)
		}
		handler.Spec.Template.Containers[0].Image = container.Status.LatestImage

		// track container for new images
		gvk := buildv1alpha1.SchemeGroupVersion.WithKind("Container")
		if err := c.tracker.Track(reconciler.MakeObjectRef(container, gvk), handler); err != nil {
			return err
		}
		return nil

	case build.FunctionRef != "":
		function, err := c.functionLister.Functions(handler.Namespace).Get(build.FunctionRef)
		if err != nil {
			return err
		}
		if function.Status.LatestImage == "" {
			return fmt.Errorf("function %q does not have a ready image", build.FunctionRef)
		}
		handler.Spec.Template.Containers[0].Image = function.Status.LatestImage

		// track function for new images
		gvk := buildv1alpha1.SchemeGroupVersion.WithKind("Function")
		if err := c.tracker.Track(reconciler.MakeObjectRef(function, gvk), handler); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("invalid handler build")
}

func (c *Reconciler) reconcileDeployment(ctx context.Context, handler *corev1alpha1.Handler, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	logger := logging.FromContext(ctx)
	desiredDeployment, err := resources.MakeDeployment(handler)
	if err != nil {
		return nil, err
	}

	if deploymentSemanticEquals(desiredDeployment, deployment) {
		// No differences to reconcile.
		return deployment, nil
	}
	diff, err := kmp.SafeDiff(desiredDeployment.Spec, deployment.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff Deployment: %v", err)
	}
	logger.Infof("Reconciling deployment diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := deployment.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredDeployment.Spec
	existing.ObjectMeta.Labels = desiredDeployment.ObjectMeta.Labels
	return c.KubeClientSet.AppsV1().Deployments(handler.Namespace).Update(existing)
}

func (c *Reconciler) createDeployment(handler *corev1alpha1.Handler) (*appsv1.Deployment, error) {
	deployment, err := resources.MakeDeployment(handler)
	if err != nil {
		return nil, err
	}
	return c.KubeClientSet.AppsV1().Deployments(handler.Namespace).Create(deployment)
}

func deploymentSemanticEquals(desiredDeployment, deployment *appsv1.Deployment) bool {
	return equality.Semantic.DeepEqual(desiredDeployment.Spec, deployment.Spec) &&
		equality.Semantic.DeepEqual(desiredDeployment.ObjectMeta.Labels, deployment.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileService(ctx context.Context, handler *corev1alpha1.Handler, service *corev1.Service) (*corev1.Service, error) {
	logger := logging.FromContext(ctx)
	desiredService, err := resources.MakeService(handler)
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
	return c.KubeClientSet.CoreV1().Services(handler.Namespace).Update(existing)
}

func (c *Reconciler) createService(handler *corev1alpha1.Handler) (*corev1.Service, error) {
	service, err := resources.MakeService(handler)
	if err != nil {
		return nil, err
	}
	return c.KubeClientSet.CoreV1().Services(handler.Namespace).Create(service)
}

func serviceSemanticEquals(desiredService, service *corev1.Service) bool {
	return equality.Semantic.DeepEqual(desiredService.Spec, service.Spec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels)
}
