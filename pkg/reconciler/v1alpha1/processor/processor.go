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
	"reflect"
	"time"

	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/logging"
	streamsv1alpha1 "github.com/projectriff/system/pkg/apis/streams/v1alpha1"
	streamsinformers "github.com/projectriff/system/pkg/client/informers/externalversions/streams/v1alpha1"
	streamslisters "github.com/projectriff/system/pkg/client/listers/streams/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/processor/resources"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
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
	processorLister streamslisters.ProcessorLister
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	processorInformer streamsinformers.ProcessorInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:            reconciler.NewBase(opt, controllerAgentName),
		processorLister: processorInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	processorInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
		DeleteFunc: impl.Enqueue,
	})

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

func (c *Reconciler) reconcile(ctx context.Context, processor *streamsv1alpha1.Processor) error {
	logger := logging.FromContext(ctx)
	if processor.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	processor.SetDefaults()

	processor.Status.InitializeConditions()

	input := processor.Spec.Inputs
	output := processor.Spec.Outputs
	function := processor.Spec.Function
	logger.Infof("Creating Processor %s with input Stream %s, Function %s, and output Stream %s", processor.Name, input, function, output)
	_, err := c.createDeployment(processor)
	if err != nil {
		logger.Warn("Failed to create Deployment", zap.Error(err))
	}
	processor.Status.ObservedGeneration = processor.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *streamsv1alpha1.Processor) (*streamsv1alpha1.Processor, error) {
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

	p, err := c.ProjectriffClientSet.StreamsV1alpha1().Processors(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(p.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("Processor %q became ready after %v", p.Name, duration)
	}

	return p, err
}

func (c *Reconciler) createDeployment(processor *streamsv1alpha1.Processor) (*appsv1.Deployment, error) {
	deployment := resources.MakeDeployment(processor)
	return c.KubeClientSet.AppsV1().Deployments(deployment.Namespace).Create(deployment)
}
