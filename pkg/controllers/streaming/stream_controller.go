/*
Copyright 2019 the original author or authors.

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

package streaming

import (
	"context"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"reflect"

	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
)

// StreamReconciler reconciles a Stream object
type StreamReconciler struct {
	client.Client
	Log                     logr.Logger
	Scheme                  *runtime.Scheme
	StreamProvisionerClient StreamProvisionerClient
}

// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=streams,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=streams/status,verbs=get;update;patch

func (r *StreamReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("stream", req.NamespacedName)

	var original streamingv1alpha1.Stream
	if err := r.Client.Get(ctx, req.NamespacedName, &original); err != nil {
		return ctrl.Result{}, ignoreNotFound(err)
	}

	// Don't modify the informers copy
	stream := original.DeepCopy()

	// Reconcile this copy of the stream and then write back any status
	// updates regardless of whether the reconciliation errored out.
	result, err := r.reconcile(ctx, log, stream)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(stream.Status, original.Status) {
		// update status
		if updateErr := r.Status().Update(ctx, &stream); updateErr != nil {
			log.Error(updateErr, "unable to update Stream status", "stream", stream)
			return ctrl.Result{Requeue: true}, updateErr
		}
	}
		if _, uErr := r.updateStatus(ctx, log, stream); uErr != nil {
			log.Error(uErr, "Failed to update stream status", "stream", stream)
			return ctrl.Result{Requeue: true}, ignoreNotFound(err)
		}
	}
	return result, err
}

func (r *StreamReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&streamingv1alpha1.Stream{}).
		Complete(r)
}

func (r *StreamReconciler) reconcile(ctx context.Context, logger logr.Logger, stream *streamingv1alpha1.Stream) (ctrl.Result, error) {
	if stream.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	stream.SetDefaults(ctx)

	stream.Status.InitializeConditions()

	// delegate to the provider via its REST API
	logger.Info("Creating Stream %s with Provider %s", stream.Name, stream.Spec.Provider)
	address, err := r.StreamProvisionerClient.CreateStream(stream.Spec.Provider, stream.Name)
	if err != nil {
		stream.Status.MarkStreamProvisionFailed(err.Error())
		return ctrl.Result{Requeue: true}, err
	}
	stream.Status.MarkStreamProvisioned()
	stream.Status.Address = *address
	stream.Status.ObservedGeneration = stream.Generation

	return ctrl.Result{}, nil
}

func (r *StreamReconciler) updateStatus(ctx context.Context, logger logr.Logger, desired *streamingv1alpha1.Stream) (*streamingv1alpha1.Stream, error) {
	desiredNamespacedName := types.NamespacedName{
		Namespace: desired.Namespace,
		Name:      desired.Name,
	}
	var stream streamingv1alpha1.Stream
	if err := r.Client.Get(ctx, desiredNamespacedName, &stream); err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(stream.Status, desired.Status) {
		return &stream, nil
	}

	becomesReady := desired.Status.IsReady() && !stream.Status.IsReady()
	// Don't modify the informers copy.
	existing := stream.DeepCopy()
	existing.Status = desired.Status

	err := r.Client.Status().Update(ctx, existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(existing.ObjectMeta.CreationTimestamp.Time)
		logger.Info("Stream %q became ready after %v", existing.Name, duration)
	}
	return existing, err
}

func ignoreNotFound(err error) error {
	if apierrs.IsNotFound(err) {
		return nil
	}
	return err
}
