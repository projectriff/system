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
	"fmt"
	"reflect"
	"time"

	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/kmp"
	"github.com/knative/pkg/logging"
	"github.com/knative/pkg/tracker"
	streamsv1alpha1 "github.com/projectriff/system/pkg/apis/streams/v1alpha1"
	streamsinformers "github.com/projectriff/system/pkg/client/informers/externalversions/streams/v1alpha1"
	streamslisters "github.com/projectriff/system/pkg/client/listers/streams/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/streamprocessor/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/streamprocessor/resources/names"
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
	ReconcilerName      = "StreamProcessors"
	controllerAgentName = "streamprocessor-controller"
)

// Reconciler implements controller.Reconciler for StreamProcessor resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	streamprocessorLister streamslisters.StreamProcessorLister
	deploymentLister      appslisters.DeploymentLister
	streamLister          streamslisters.StreamLister

	tracker tracker.Interface
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	streamprocessorInformer streamsinformers.StreamProcessorInformer,
	deploymentInformer appsinformers.DeploymentInformer,
	streamInformer streamsinformers.StreamInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:                  reconciler.NewBase(opt, controllerAgentName),
		streamprocessorLister: streamprocessorInformer.Lister(),
		deploymentLister:      deploymentInformer.Lister(),
		streamLister:          streamInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	streamprocessorInformer.Informer().AddEventHandler(reconciler.Handler(impl.Enqueue))

	// controlled resources
	deploymentInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(streamsv1alpha1.SchemeGroupVersion.WithKind("StreamProcessor")),
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})

	// referenced resources
	c.tracker = tracker.New(impl.EnqueueKey, opt.GetTrackerLease())
	streamInformer.Informer().AddEventHandler(reconciler.Handler(
		// Call the tracker's OnChanged method, but we've seen the objects
		// coming through this path missing TypeMeta, so ensure it is properly
		// populated.
		controller.EnsureTypeMeta(
			c.tracker.OnChanged,
			streamsv1alpha1.SchemeGroupVersion.WithKind("Stream"),
		),
	))

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

	// resolve input addresses
	for _, inputName := range streamprocessor.Spec.Inputs {
		input, err := c.streamLister.Streams(streamprocessor.Namespace).Get(inputName)
		if err != nil {
			return err
		}
		gvk := streamsv1alpha1.SchemeGroupVersion.WithKind("Stream")
		if err := c.tracker.Track(reconciler.MakeObjectRef(input, gvk), streamprocessor); err != nil {
			return err
		}
		inputAddresses = append(inputAddresses, input.Status.Address.String())
	}
	streamprocessor.Status.InputAddresses = inputAddresses

	// resolve output addresses
	for _, outputName := range streamprocessor.Spec.Outputs {
		output, err := c.streamLister.Streams(streamprocessor.Namespace).Get(outputName)
		if err != nil {
			return err
		}
		gvk := streamsv1alpha1.SchemeGroupVersion.WithKind("Stream")
		if err := c.tracker.Track(reconciler.MakeObjectRef(output, gvk), streamprocessor); err != nil {
			return err
		}
		outputAddresses = append(outputAddresses, output.Status.Address.String())
	}
	streamprocessor.Status.OutputAddresses = outputAddresses

	deploymentName := resourcenames.Deployment(streamprocessor)
	deployment, err := c.deploymentLister.Deployments(streamprocessor.Namespace).Get(deploymentName)
	if errors.IsNotFound(err) {
		deployment, err = c.createDeployment(streamprocessor)
		if err != nil {
			logger.Errorf("Failed to create Deployment %q: %v", deploymentName, err)
			c.Recorder.Eventf(streamprocessor, corev1.EventTypeWarning, "CreationFailed", "Failed to create Deployment %q: %v", deploymentName, err)
			return err
		}
		if deployment != nil {
			c.Recorder.Eventf(streamprocessor, corev1.EventTypeNormal, "Created", "Created Deployment %q", deploymentName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile StreamProcessor: %q failed to Get Deployment: %q; %v", streamprocessor.Name, deploymentName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(deployment, streamprocessor) {
		// Surface an error in the streamprocessor's status,and return an error.
		streamprocessor.Status.MarkDeploymentNotOwned(deploymentName)
		return fmt.Errorf("StreamProcessor: %q does not own Deployment: %q", streamprocessor.Name, deploymentName)
	} else {
		deployment, err = c.reconcileDeployment(ctx, streamprocessor, deployment)
		if err != nil {
			logger.Errorf("Failed to reconcile StreamProcessor: %q failed to reconcile Deployment: %q; %v", streamprocessor.Name, deployment, zap.Error(err))
			return err
		}
		if deployment == nil {
			c.Recorder.Eventf(streamprocessor, corev1.EventTypeNormal, "Deleted", "Deleted Deployment %q", deploymentName)
		}
	}

	// Update our Status based on the state of our underlying Deployment.
	streamprocessor.Status.DeploymentName = deployment.Name
	streamprocessor.Status.PropagateDeploymentStatus(&deployment.Status)

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

func (c *Reconciler) reconcileDeployment(ctx context.Context, streamprocessor *streamsv1alpha1.StreamProcessor, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	logger := logging.FromContext(ctx)
	desiredDeployment, err := resources.MakeDeployment(streamprocessor)
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
	return c.KubeClientSet.AppsV1().Deployments(streamprocessor.Namespace).Update(existing)
}

func (c *Reconciler) createDeployment(streamprocessor *streamsv1alpha1.StreamProcessor) (*appsv1.Deployment, error) {
	deployment, err := resources.MakeDeployment(streamprocessor)
	if err != nil {
		return nil, err
	}
	if deployment == nil {
		// nothing to create
		return deployment, nil
	}
	return c.KubeClientSet.AppsV1().Deployments(streamprocessor.Namespace).Create(deployment)
}

func deploymentSemanticEquals(desiredDeployment, deployment *appsv1.Deployment) bool {
	return equality.Semantic.DeepEqual(desiredDeployment.Spec, deployment.Spec) &&
		equality.Semantic.DeepEqual(desiredDeployment.ObjectMeta.Labels, deployment.ObjectMeta.Labels)
}
