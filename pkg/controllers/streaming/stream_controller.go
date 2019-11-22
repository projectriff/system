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
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
	"github.com/projectriff/system/pkg/controllers"
)

const (
	bindingMetadataIndexField = ".metadata.bindingMetadataController"
	bindingSecretIndexField   = ".metadata.bindingSecretController"
)

// StreamReconciler reconciles a Stream object
type StreamReconciler struct {
	client.Client
	Log                     logr.Logger
	Scheme                  *runtime.Scheme
	StreamProvisionerClient StreamProvisionerClient
}

// For
// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=streams,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=streams/status,verbs=get;update;patch
// Owns
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

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
		log.Info("updating stream status", "diff", cmp.Diff(original.Status, stream.Status))
		if updateErr := r.Status().Update(ctx, stream); updateErr != nil {
			log.Error(updateErr, "unable to update Stream status", "stream", stream)
			return ctrl.Result{Requeue: true}, updateErr
		}
	}
	return result, err
}

func (r *StreamReconciler) reconcile(ctx context.Context, log logr.Logger, stream *streamingv1alpha1.Stream) (ctrl.Result, error) {
	if stream.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	stream.Default()

	stream.Status.InitializeConditions()

	// delegate to the provider via its REST API
	log.Info("calling provisioner for Stream", "provisioner", stream.Spec.Provider)
	address, err := r.StreamProvisionerClient.ProvisionStream(stream)
	if err != nil {
		stream.Status.MarkStreamProvisionFailed(err.Error())
		return ctrl.Result{Requeue: true}, err
	}
	stream.Status.MarkStreamProvisioned()
	stream.Status.Address = *address

	// reconcile binding metadata
	childBindingMetadata, err := r.reconcileChildBindingMetadata(ctx, log, stream)
	if err != nil {
		log.Error(err, "unable to reconcile child binding metadata ConfigMap", "stream", stream)
		return ctrl.Result{}, err
	}
	stream.Status.Binding.MetadataRef = corev1.LocalObjectReference{}
	if childBindingMetadata != nil {
		stream.Status.Binding.MetadataRef.Name = childBindingMetadata.Name
	}

	// reconcile binding secret
	childBindingSecret, err := r.reconcileChildBindingSecret(ctx, log, stream)
	if err != nil {
		log.Error(err, "unable to reconcile child binding secret Secret", "stream", stream)
		return ctrl.Result{}, err
	}
	stream.Status.Binding.SecretRef = corev1.LocalObjectReference{}
	if childBindingSecret != nil {
		stream.Status.Binding.SecretRef.Name = childBindingSecret.Name
	}

	if childBindingMetadata == nil {
		stream.Status.MarkBindingNotReady("binding metadata not available")
	} else if childBindingSecret == nil {
		stream.Status.MarkBindingNotReady("binding secret not available")
	} else {
		stream.Status.MarkBindingReady()
	}

	stream.Status.ObservedGeneration = stream.Generation

	return ctrl.Result{}, nil
}

func (r *StreamReconciler) reconcileChildBindingMetadata(ctx context.Context, log logr.Logger, stream *streamingv1alpha1.Stream) (*corev1.ConfigMap, error) {
	var actualBindingMetadata corev1.ConfigMap
	var childBindingMetadatas corev1.ConfigMapList
	if err := r.List(ctx, &childBindingMetadatas, client.InNamespace(stream.Namespace), client.MatchingField(bindingMetadataIndexField, stream.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childBindingMetadatas.Items) == 1 {
		actualBindingMetadata = childBindingMetadatas.Items[0]
	} else if len(childBindingMetadatas.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraBindingMetadata := range childBindingMetadatas.Items {
			log.Info("deleting extra binding metadata", "metadata", extraBindingMetadata)
			if err := r.Delete(ctx, &extraBindingMetadata); err != nil {
				return nil, err
			}
		}
	}

	desiredBindingMetadata, err := r.constructBindingMetadata(stream)
	if err != nil {
		return nil, err
	}

	// delete binding metadata if no longer needed
	if desiredBindingMetadata == nil {
		log.Info("deleting binding metadata", "metadata", actualBindingMetadata)
		if err := r.Delete(ctx, &actualBindingMetadata); err != nil {
			log.Error(err, "unable to delete binding metadata ConfigMap for Stream", "metadata", actualBindingMetadata)
			return nil, err
		}
		return nil, nil
	}

	// create binding metadata if it doesn't exist
	if actualBindingMetadata.CreationTimestamp.IsZero() {
		log.Info("creating binding metadata", "data", desiredBindingMetadata.Data)
		if err := r.Create(ctx, desiredBindingMetadata); err != nil {
			log.Error(err, "unable to create binding metadata ConfigMap for Stream", "metadata", desiredBindingMetadata)
			return nil, err
		}
		return desiredBindingMetadata, nil
	}

	if r.bindingMetadataSemanticEquals(desiredBindingMetadata, &actualBindingMetadata) {
		// binding metadata is unchanged
		return &actualBindingMetadata, nil
	}

	// update binding metadata with desired changes
	bindingMetadata := actualBindingMetadata.DeepCopy()
	bindingMetadata.ObjectMeta.Labels = desiredBindingMetadata.ObjectMeta.Labels
	bindingMetadata.Data = desiredBindingMetadata.Data
	log.Info("reconciling binding metadata", "diff", cmp.Diff(actualBindingMetadata.Data, bindingMetadata.Data))
	if err := r.Update(ctx, bindingMetadata); err != nil {
		log.Error(err, "unable to update binding metadata ConfigMap for Stream", "metadata", bindingMetadata)
		return nil, err
	}

	return bindingMetadata, nil
}

func (r *StreamReconciler) bindingMetadataSemanticEquals(desiredBindingMetadata, bindingMetadata *corev1.ConfigMap) bool {
	return equality.Semantic.DeepEqual(desiredBindingMetadata.Data, bindingMetadata.Data) &&
		equality.Semantic.DeepEqual(desiredBindingMetadata.ObjectMeta.Labels, bindingMetadata.ObjectMeta.Labels)
}

func (r *StreamReconciler) constructBindingMetadata(stream *streamingv1alpha1.Stream) (*corev1.ConfigMap, error) {
	metadata := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				streamingv1alpha1.StreamLabelKey: stream.Name,
			},
			Annotations: make(map[string]string),
			Name:        fmt.Sprintf("%s-stream-binding-metadata", stream.Name),
			Namespace:   stream.Namespace,
		},
		Data: map[string]string{
			// spec required values
			"kind":     (&streamingv1alpha1.Stream{}).GetGroupVersionKind().GroupKind().String(),
			"provider": "riff Streaming",
			"tags":     "",
			// non-spec values
			"stream":      stream.Name,
			"contentType": stream.Spec.ContentType,
		},
	}
	if err := ctrl.SetControllerReference(stream, metadata, r.Scheme); err != nil {
		return nil, err
	}

	return metadata, nil
}

func (r *StreamReconciler) reconcileChildBindingSecret(ctx context.Context, log logr.Logger, stream *streamingv1alpha1.Stream) (*corev1.Secret, error) {
	var actualBindingSecret corev1.Secret
	var childBindingSecrets corev1.SecretList
	if err := r.List(ctx, &childBindingSecrets, client.InNamespace(stream.Namespace), client.MatchingField(bindingSecretIndexField, stream.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childBindingSecrets.Items) == 1 {
		actualBindingSecret = childBindingSecrets.Items[0]
	} else if len(childBindingSecrets.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraBindingSecret := range childBindingSecrets.Items {
			log.Info("deleting extra binding secret", "secret", extraBindingSecret.Name)
			if err := r.Delete(ctx, &extraBindingSecret); err != nil {
				return nil, err
			}
		}
	}

	desiredBindingSecret, err := r.constructBindingSecret(stream)
	if err != nil {
		return nil, err
	}

	// delete binding secret if no longer needed
	if desiredBindingSecret == nil {
		log.Info("deleting binding secret", "secret", actualBindingSecret.Name)
		if err := r.Delete(ctx, &actualBindingSecret); err != nil {
			log.Error(err, "unable to delete binding secret Secret for Stream", "secret", actualBindingSecret.Name)
			return nil, err
		}
		return nil, nil
	}

	// create binding secret if it doesn't exist
	if actualBindingSecret.CreationTimestamp.IsZero() {
		log.Info("creating binding secret", "secret", desiredBindingSecret.Name)
		if err := r.Create(ctx, desiredBindingSecret); err != nil {
			log.Error(err, "unable to create binding secret Secret for Stream", "secret", desiredBindingSecret.Name)
			return nil, err
		}
		return desiredBindingSecret, nil
	}

	if r.bindingSecretSemanticEquals(desiredBindingSecret, &actualBindingSecret) {
		// binding secret is unchanged
		return &actualBindingSecret, nil
	}

	// update binding secret with desired changes
	bindingSecret := actualBindingSecret.DeepCopy()
	bindingSecret.ObjectMeta.Labels = desiredBindingSecret.ObjectMeta.Labels
	bindingSecret.StringData = desiredBindingSecret.StringData
	log.Info("reconciling binding secret")
	if err := r.Update(ctx, bindingSecret); err != nil {
		log.Error(err, "unable to update binding secret Secret for Stream", "secret", bindingSecret.Name)
		return nil, err
	}

	return bindingSecret, nil
}

func (r *StreamReconciler) bindingSecretSemanticEquals(desiredBindingSecret, secret *corev1.Secret) bool {
	return equality.Semantic.DeepEqual(desiredBindingSecret.StringData, secret.StringData) &&
		equality.Semantic.DeepEqual(desiredBindingSecret.ObjectMeta.Labels, secret.ObjectMeta.Labels)
}

func (r *StreamReconciler) constructBindingSecret(stream *streamingv1alpha1.Stream) (*corev1.Secret, error) {
	if stream.Status.Address.Gateway == "" || stream.Status.Address.Topic == "" {
		return nil, nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				streamingv1alpha1.StreamLabelKey: stream.Name,
			},
			Annotations: make(map[string]string),
			Name:        fmt.Sprintf("%s-stream-binding-secret", stream.Name),
			Namespace:   stream.Namespace,
		},
		StringData: map[string]string{
			"gateway": stream.Status.Address.Gateway,
			"topic":   stream.Status.Address.Topic,
		},
	}
	if err := ctrl.SetControllerReference(stream, secret, r.Scheme); err != nil {
		return nil, err
	}

	return secret, nil
}

func (r *StreamReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := controllers.IndexControllersOfType(mgr, bindingMetadataIndexField, &streamingv1alpha1.Stream{}, &corev1.ConfigMap{}); err != nil {
		return err
	}
	if err := controllers.IndexControllersOfType(mgr, bindingSecretIndexField, &streamingv1alpha1.Stream{}, &corev1.Secret{}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&streamingv1alpha1.Stream{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
