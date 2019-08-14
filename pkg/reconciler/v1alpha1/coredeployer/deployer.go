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

package coredeployer

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
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/coredeployer/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/coredeployer/resources/names"
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
	ReconcilerName      = "Deployers"
	controllerAgentName = "deployer-controller"
)

// Reconciler implements controller.Reconciler for Deployer resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	deployerLister    coreruntimelisters.DeployerLister
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
	deployerInformer coreruntimeinformers.DeployerInformer,
	deploymentInformer appsinformers.DeploymentInformer,
	serviceInformer coreinformers.ServiceInformer,
	applicationInformer buildinformers.ApplicationInformer,
	containerInformer buildinformers.ContainerInformer,
	functionInformer buildinformers.FunctionInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:              reconciler.NewBase(opt, controllerAgentName),
		deployerLister:    deployerInformer.Lister(),
		deploymentLister:  deploymentInformer.Lister(),
		serviceLister:     serviceInformer.Lister(),
		applicationLister: applicationInformer.Lister(),
		containerLister:   containerInformer.Lister(),
		functionLister:    functionInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName)

	c.Logger.Info("Setting up event handlers")
	deployerInformer.Informer().AddEventHandler(reconciler.Handler(impl.Enqueue))

	// controlled resources
	deploymentInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(corev1alpha1.SchemeGroupVersion.WithKind("Deployer")),
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})
	serviceInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(corev1alpha1.SchemeGroupVersion.WithKind("Deployer")),
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

func (c *Reconciler) reconcile(ctx context.Context, deployer *corev1alpha1.Deployer) error {
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

	deploymentName := resourcenames.Deployment(deployer)
	deployment, err := c.deploymentLister.Deployments(deployer.Namespace).Get(deploymentName)
	if errors.IsNotFound(err) {
		deployment, err = c.createDeployment(deployer)
		if err != nil {
			logger.Errorf("Failed to create Deployment %q: %v", deploymentName, err)
			c.Recorder.Eventf(deployer, corev1.EventTypeWarning, "CreationFailed", "Failed to create Deployment %q: %v", deploymentName, err)
			return err
		}
		if deployment != nil {
			c.Recorder.Eventf(deployer, corev1.EventTypeNormal, "Created", "Created Deployment %q", deploymentName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Deployer: %q failed to Get Deployment: %q; %v", deployer.Name, deploymentName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(deployment, deployer) {
		// Surface an error in the deployer's status,and return an error.
		deployer.Status.MarkDeploymentNotOwned(deploymentName)
		return fmt.Errorf("Deployer: %q does not own Deployment: %q", deployer.Name, deploymentName)
	} else {
		deployment, err = c.reconcileDeployment(ctx, deployer, deployment)
		if err != nil {
			logger.Errorf("Failed to reconcile Deployer: %q failed to reconcile Deployment: %q; %v", deployer.Name, deployment, zap.Error(err))
			return err
		}
	}

	// Update our Status based on the state of our underlying Deployment.
	deployer.Status.DeploymentName = deployment.Name
	deployer.Status.PropagateDeploymentStatus(&deployment.Status)

	serviceName := resourcenames.Service(deployer)
	service, err := c.serviceLister.Services(deployer.Namespace).Get(serviceName)
	if errors.IsNotFound(err) {
		service, err = c.createService(deployer)
		if err != nil {
			logger.Errorf("Failed to create Service %q: %v", serviceName, err)
			c.Recorder.Eventf(deployer, corev1.EventTypeWarning, "CreationFailed", "Failed to create Service %q: %v", serviceName, err)
			return err
		}
		if service != nil {
			c.Recorder.Eventf(deployer, corev1.EventTypeNormal, "Created", "Created Service %q", serviceName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Deployer: %q failed to Get Service: %q; %v", deployer.Name, serviceName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(service, deployer) {
		// Surface an error in the deployer's status,and return an error.
		deployer.Status.MarkServiceNotOwned(serviceName)
		return fmt.Errorf("Deployer: %q does not own Service: %q", deployer.Name, serviceName)
	} else if service, err = c.reconcileService(ctx, deployer, service); err != nil {
		logger.Errorf("Failed to reconcile Deployer: %q failed to reconcile Service: %q; %v", deployer.Name, service, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Service.
	deployer.Status.ServiceName = service.Name
	deployer.Status.PropagateServiceStatus(&service.Status)

	deployer.Status.ObservedGeneration = deployer.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *corev1alpha1.Deployer) (*corev1alpha1.Deployer, error) {
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

	h, err := c.ProjectriffClientSet.CoreV1alpha1().Deployers(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(h.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("Deployer %q became ready after %v", deployer.Name, duration)
	}

	return h, err
}

func (c *Reconciler) reconcileBuild(deployer *corev1alpha1.Deployer) error {
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

func (c *Reconciler) reconcileDeployment(ctx context.Context, deployer *corev1alpha1.Deployer, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	logger := logging.FromContext(ctx)
	desiredDeployment, err := resources.MakeDeployment(deployer)
	if err != nil {
		return nil, err
	}

	// copy spec fields that should be preserved
	desiredDeployment.Spec.Replicas = deployment.Spec.Replicas

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
	return c.KubeClientSet.AppsV1().Deployments(deployer.Namespace).Update(existing)
}

func (c *Reconciler) createDeployment(deployer *corev1alpha1.Deployer) (*appsv1.Deployment, error) {
	deployment, err := resources.MakeDeployment(deployer)
	if err != nil {
		return nil, err
	}
	return c.KubeClientSet.AppsV1().Deployments(deployer.Namespace).Create(deployment)
}

func deploymentSemanticEquals(desiredDeployment, deployment *appsv1.Deployment) bool {
	return equality.Semantic.DeepEqual(desiredDeployment.Spec, deployment.Spec) &&
		equality.Semantic.DeepEqual(desiredDeployment.ObjectMeta.Labels, deployment.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileService(ctx context.Context, deployer *corev1alpha1.Deployer, service *corev1.Service) (*corev1.Service, error) {
	logger := logging.FromContext(ctx)
	desiredService, err := resources.MakeService(deployer)
	if err != nil {
		return nil, err
	}

	// copy spec fields that should be preserved
	desiredService.Spec.ClusterIP = service.Spec.ClusterIP

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
	return c.KubeClientSet.CoreV1().Services(deployer.Namespace).Update(existing)
}

func (c *Reconciler) createService(deployer *corev1alpha1.Deployer) (*corev1.Service, error) {
	service, err := resources.MakeService(deployer)
	if err != nil {
		return nil, err
	}
	return c.KubeClientSet.CoreV1().Services(deployer.Namespace).Create(service)
}

func serviceSemanticEquals(desiredService, service *corev1.Service) bool {
	return equality.Semantic.DeepEqual(desiredService.Spec, service.Spec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels)
}
