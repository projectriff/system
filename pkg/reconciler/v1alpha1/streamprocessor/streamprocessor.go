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

package streamprocessor

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
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/streamprocessor/resources"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
)

const (
	// ReconcilerName is the name of the reconciler
	ReconcilerName      = "StreamProcessors"
	controllerAgentName = "streamprocessor-controller"
)

// Reconciler implements controller.Reconciler for StreamProcessor resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	streamprocessorLister streamslisters.StreamProcessorLister
	streamLister          streamslisters.StreamLister
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	streamprocessorInformer streamsinformers.StreamProcessorInformer,
	streamInformer streamsinformers.StreamInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:                  reconciler.NewBase(opt, controllerAgentName),
		streamprocessorLister: streamprocessorInformer.Lister(),
		streamLister:          streamInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	streamprocessorInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
		DeleteFunc: impl.Enqueue,
	})

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the StreamProcessor resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the StreamProcessor resource with this namespace/name
	original, err := c.streamprocessorLister.StreamProcessors(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("streamprocessor %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	streamprocessor := original.DeepCopy()

	// Reconcile this copy of the streamprocessor and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, streamprocessor)

	if equality.Semantic.DeepEqual(original.Status, streamprocessor.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(streamprocessor); uErr != nil {
		logger.Warn("Failed to update streamprocessor status", zap.Error(uErr))
		c.Recorder.Eventf(streamprocessor, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for StreamProcessor %q: %v", streamprocessor.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(streamprocessor, corev1.EventTypeNormal, "Updated", "Updated StreamProcessor %q", streamprocessor.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, streamprocessor *streamsv1alpha1.StreamProcessor) error {
	logger := logging.FromContext(ctx)
	if streamprocessor.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	streamprocessor.SetDefaults(ctx)

	streamprocessor.Status.InitializeConditions()

	var inputAddresses, outputAddresses []string = nil, nil

	for _, inputName := range streamprocessor.Spec.Inputs {
		input, err := c.streamLister.Streams(streamprocessor.Namespace).Get(inputName)
		if err != nil {
			return err
		}
		inputAddresses = append(inputAddresses, input.Status.Address.String())
	}
	streamprocessor.Status.InputAddresses = inputAddresses

	for _, outputName := range streamprocessor.Spec.Outputs {
		output, err := c.streamLister.Streams(streamprocessor.Namespace).Get(outputName)
		if err != nil {
			return err
		}
		outputAddresses = append(outputAddresses, output.Status.Address.String())
	}
	streamprocessor.Status.OutputAddresses = outputAddresses

	logger.Infof("Creating StreamProcessor %s with input Streams %s, Function %s, and output Streams %s", streamprocessor.Name, inputAddresses, streamprocessor.Spec.Function, outputAddresses)
	_, err := c.createDeployment(streamprocessor)
	if err != nil {
		logger.Warn("Failed to create Deployment", zap.Error(err))
	}
	streamprocessor.Status.ObservedGeneration = streamprocessor.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *streamsv1alpha1.StreamProcessor) (*streamsv1alpha1.StreamProcessor, error) {
	streamprocessor, err := c.streamprocessorLister.StreamProcessors(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(streamprocessor.Status, desired.Status) {
		return streamprocessor, nil
	}
	becomesReady := desired.Status.IsReady() && !streamprocessor.Status.IsReady()
	// Don't modify the informers copy.
	existing := streamprocessor.DeepCopy()
	existing.Status = desired.Status

	p, err := c.ProjectriffClientSet.StreamsV1alpha1().StreamProcessors(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(p.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("StreamProcessor %q became ready after %v", p.Name, duration)
	}

	return p, err
}

func (c *Reconciler) createDeployment(streamprocessor *streamsv1alpha1.StreamProcessor) (*appsv1.Deployment, error) {
	deployment := resources.MakeDeployment(streamprocessor)
	return c.KubeClientSet.AppsV1().Deployments(deployment.Namespace).Create(deployment)
}
