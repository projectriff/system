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

package processor

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
	streamv1alpha1 "github.com/projectriff/system/pkg/apis/stream/v1alpha1"
	buildinformers "github.com/projectriff/system/pkg/client/informers/externalversions/build/v1alpha1"
	streaminformers "github.com/projectriff/system/pkg/client/informers/externalversions/stream/v1alpha1"
	buildlisters "github.com/projectriff/system/pkg/client/listers/build/v1alpha1"
	streamlisters "github.com/projectriff/system/pkg/client/listers/stream/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/processor/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/processor/resources/names"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	// ReconcilerName is the name of the reconciler
	ReconcilerName      = "Processors"
	controllerAgentName = "processor-controller"
)

// Reconciler implements controller.Reconciler for Processor resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	processorLister  streamlisters.ProcessorLister
	functionLister   buildlisters.FunctionLister
	streamLister     streamlisters.StreamLister
	deploymentLister appslisters.DeploymentLister

	tracker tracker.Interface
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	processorInformer streaminformers.ProcessorInformer,
	functionInformer buildinformers.FunctionInformer,
	streamInformer streaminformers.StreamInformer,
	deploymentInformer appsinformers.DeploymentInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:             reconciler.NewBase(opt, controllerAgentName),
		processorLister:  processorInformer.Lister(),
		functionLister:   functionInformer.Lister(),
		streamLister:     streamInformer.Lister(),
		deploymentLister: deploymentInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	processorInformer.Informer().AddEventHandler(reconciler.Handler(impl.Enqueue))

	// controlled resources
	deploymentInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(streamv1alpha1.SchemeGroupVersion.WithKind("Processor")),
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})

	// referenced resources
	c.tracker = tracker.New(impl.EnqueueKey, opt.GetTrackerLease())
	functionInformer.Informer().AddEventHandler(reconciler.Handler(
		// Call the tracker's OnChanged method, but we've seen the objects
		// coming through this path missing TypeMeta, so ensure it is properly
		// populated.
		controller.EnsureTypeMeta(
			c.tracker.OnChanged,
			buildv1alpha1.SchemeGroupVersion.WithKind("Function"),
		),
	))
	streamInformer.Informer().AddEventHandler(reconciler.Handler(
		// Call the tracker's OnChanged method, but we've seen the objects
		// coming through this path missing TypeMeta, so ensure it is properly
		// populated.
		controller.EnsureTypeMeta(
			c.tracker.OnChanged,
			streamv1alpha1.SchemeGroupVersion.WithKind("Stream"),
		),
	))

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Processor resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the Processor resource with this namespace/name
	original, err := c.processorLister.Processors(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("processor %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	processor := original.DeepCopy()

	// Reconcile this copy of the processor and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, processor)

	if equality.Semantic.DeepEqual(original.Status, processor.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(processor); uErr != nil {
		logger.Warn("Failed to update processor status", zap.Error(uErr))
		c.Recorder.Eventf(processor, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for Processor %q: %v", processor.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(processor, corev1.EventTypeNormal, "Updated", "Updated Processor %q", processor.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, processor *streamv1alpha1.Processor) error {
	logger := logging.FromContext(ctx)
	if processor.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	processor.SetDefaults(ctx)

	processor.Status.InitializeConditions()

	// resolve function
	function, err := c.functionLister.Functions(processor.Namespace).Get(processor.Spec.FunctionRef)
	if err != nil {
		if errors.IsNotFound(err) {
			processor.Status.MarkFunctionNotFound(processor.Spec.FunctionRef)
		}
		return err
	}
	gvk := buildv1alpha1.SchemeGroupVersion.WithKind("Function")
	if err := c.tracker.Track(reconciler.MakeObjectRef(function, gvk), processor); err != nil {
		return err
	}
	processor.Status.PropagateFunctionStatus(&function.Status)

	var inputAddresses, outputAddresses []string = nil, nil

	// resolve input addresses
	for _, inputName := range processor.Spec.Inputs {
		input, err := c.streamLister.Streams(processor.Namespace).Get(inputName)
		if err != nil {
			return err
		}
		gvk := streamv1alpha1.SchemeGroupVersion.WithKind("Stream")
		if err := c.tracker.Track(reconciler.MakeObjectRef(input, gvk), processor); err != nil {
			return err
		}
		inputAddresses = append(inputAddresses, input.Status.Address.String())
	}
	processor.Status.InputAddresses = inputAddresses

	// resolve output addresses and content-types
	outputContentTypes := make([]string, len(processor.Spec.Outputs))
	for i, outputName := range processor.Spec.Outputs {
		output, err := c.streamLister.Streams(processor.Namespace).Get(outputName)
		if err != nil {
			return err
		}
		gvk := streamv1alpha1.SchemeGroupVersion.WithKind("Stream")
		if err := c.tracker.Track(reconciler.MakeObjectRef(output, gvk), processor); err != nil {
			return err
		}
		outputAddresses = append(outputAddresses, output.Status.Address.String())
		outputContentTypes[i] = output.Spec.ContentType
	}
	processor.Status.OutputAddresses = outputAddresses
	processor.Status.OutputContentTypes = outputContentTypes

	deploymentName := resourcenames.Deployment(processor)
	deployment, err := c.deploymentLister.Deployments(processor.Namespace).Get(deploymentName)
	if errors.IsNotFound(err) {
		deployment, err = c.createDeployment(processor)
		if err != nil {
			logger.Errorf("Failed to create Deployment %q: %v", deploymentName, err)
			c.Recorder.Eventf(processor, corev1.EventTypeWarning, "CreationFailed", "Failed to create Deployment %q: %v", deploymentName, err)
			return err
		}
		if deployment != nil {
			c.Recorder.Eventf(processor, corev1.EventTypeNormal, "Created", "Created Deployment %q", deploymentName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Processor: %q failed to Get Deployment: %q; %v", processor.Name, deploymentName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(deployment, processor) {
		// Surface an error in the processor's status,and return an error.
		processor.Status.MarkDeploymentNotOwned(deploymentName)
		return fmt.Errorf("Processor: %q does not own Deployment: %q", processor.Name, deploymentName)
	} else {
		deployment, err = c.reconcileDeployment(ctx, processor, deployment)
		if err != nil {
			logger.Errorf("Failed to reconcile Processor: %q failed to reconcile Deployment: %q; %v", processor.Name, deployment, zap.Error(err))
			return err
		}
		if deployment == nil {
			c.Recorder.Eventf(processor, corev1.EventTypeNormal, "Deleted", "Deleted Deployment %q", deploymentName)
		}
	}

	// Update our Status based on the state of our underlying Deployment.
	processor.Status.DeploymentName = deployment.Name
	processor.Status.PropagateDeploymentStatus(&deployment.Status)

	processor.Status.ObservedGeneration = processor.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *streamv1alpha1.Processor) (*streamv1alpha1.Processor, error) {
	processor, err := c.processorLister.Processors(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(processor.Status, desired.Status) {
		return processor, nil
	}
	becomesReady := desired.Status.IsReady() && !processor.Status.IsReady()
	// Don't modify the informers copy.
	existing := processor.DeepCopy()
	existing.Status = desired.Status

	p, err := c.ProjectriffClientSet.StreamV1alpha1().Processors(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(p.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("Processor %q became ready after %v", p.Name, duration)
	}

	return p, err
}

func (c *Reconciler) reconcileDeployment(ctx context.Context, processor *streamv1alpha1.Processor, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	logger := logging.FromContext(ctx)
	desiredDeployment, err := resources.MakeDeployment(processor)
	if err != nil {
		return nil, err
	}

	// Preserve replicas as is it likely set by an autoscaler
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
	return c.KubeClientSet.AppsV1().Deployments(processor.Namespace).Update(existing)
}

func (c *Reconciler) createDeployment(processor *streamv1alpha1.Processor) (*appsv1.Deployment, error) {
	deployment, err := resources.MakeDeployment(processor)
	if err != nil {
		return nil, err
	}
	if deployment == nil {
		// nothing to create
		return deployment, nil
	}
	return c.KubeClientSet.AppsV1().Deployments(processor.Namespace).Create(deployment)
}

func deploymentSemanticEquals(desiredDeployment, deployment *appsv1.Deployment) bool {
	return equality.Semantic.DeepEqual(desiredDeployment.Spec, deployment.Spec) &&
		equality.Semantic.DeepEqual(desiredDeployment.ObjectMeta.Labels, deployment.ObjectMeta.Labels)
}
