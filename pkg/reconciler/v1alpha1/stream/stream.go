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

package stream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/logging"
	streamv1alpha1 "github.com/projectriff/system/pkg/apis/stream/v1alpha1"
	streaminformers "github.com/projectriff/system/pkg/client/informers/externalversions/stream/v1alpha1"
	streamlisters "github.com/projectriff/system/pkg/client/listers/stream/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
)

const (
	// ReconcilerName is the name of the reconciler
	ReconcilerName      = "Streams"
	controllerAgentName = "stream-controller"
)

// Reconciler implements controller.Reconciler for Stream resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	streamLister streamlisters.StreamLister
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)
var httpClient = http.DefaultClient

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	streamInformer streaminformers.StreamInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:         reconciler.NewBase(opt, controllerAgentName),
		streamLister: streamInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	streamInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
		DeleteFunc: impl.Enqueue,
	})

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Stream resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the Stream resource with this namespace/name
	original, err := c.streamLister.Streams(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("stream %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	stream := original.DeepCopy()

	// Reconcile this copy of the stream and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, stream)

	if equality.Semantic.DeepEqual(original.Status, stream.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(stream); uErr != nil {
		logger.Warn("Failed to update stream status", zap.Error(uErr))
		c.Recorder.Eventf(stream, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for Stream %q: %v", stream.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(stream, corev1.EventTypeNormal, "Updated", "Updated Stream %q", stream.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, stream *streamv1alpha1.Stream) error {
	logger := logging.FromContext(ctx)
	if stream.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	stream.SetDefaults(ctx)

	stream.Status.InitializeConditions()

	// delegate to the provider via its REST API
	logger.Infof("Creating Stream %s with Provider %s", stream.Name, stream.Spec.Provider)
	address, err := createStream(stream.Spec.Provider, stream.Name)
	if err != nil {
		stream.Status.MarkStreamProvisionFailed(err.Error())
		return err
	}
	stream.Status.MarkStreamProvisioned()
	stream.Status.Address = *address
	stream.Status.ObservedGeneration = stream.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *streamv1alpha1.Stream) (*streamv1alpha1.Stream, error) {
	stream, err := c.streamLister.Streams(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(stream.Status, desired.Status) {
		return stream, nil
	}
	becomesReady := desired.Status.IsReady() && !stream.Status.IsReady()
	// Don't modify the informers copy.
	existing := stream.DeepCopy()
	existing.Status = desired.Status

	s, err := c.ProjectriffClientSet.StreamV1alpha1().Streams(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(s.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("Stream %q became ready after %v", s.Name, duration)
	}

	return s, err
}

func createStream(provider string, stream string) (*streamv1alpha1.StreamAddress, error) {
	url := fmt.Sprintf("http://%s.default.svc.cluster.local/%s", provider, stream)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader([]byte{}))
	if err != nil {
		return nil, err
	}
	//req.Header.Add("content-type", "application/json")
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("status: %d", res.StatusCode)
	}
	address := &streamv1alpha1.StreamAddress{}
	json.NewDecoder(res.Body).Decode(address)
	return address, nil
}
