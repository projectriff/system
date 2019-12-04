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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/projectriff/system/pkg/apis"
	kedav1alpha1 "github.com/projectriff/system/pkg/apis/thirdparty/keda/v1alpha1"
	"github.com/projectriff/system/pkg/controllers"

	"github.com/projectriff/system/pkg/apis/build/v1alpha1"
	"github.com/projectriff/system/pkg/tracker"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
)

const (
	processorDeploymentIndexField   = ".metadata.processorDeploymentController"
	processorScaledObjectIndexField = ".metadata.processorScaledObjectController"
)

const (
	bindingsRootPath = "/var/riff/bindings"
)

// ProcessorReconciler reconciles a Processor object
type ProcessorReconciler struct {
	client.Client
	Log       logr.Logger
	Scheme    *runtime.Scheme
	Tracker   tracker.Tracker
	Namespace string
}

// For
// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=processors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=processors/status,verbs=get;update;patch
// Owns
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keda.k8s.io,resources=scaledobjects,verbs=get;list;watch;create;update;patch;delete
// Watches
// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=streams,verbs=get;watch
// +kubebuilder:rbac:groups=build.projectriff.io,resources=containers;functions,verbs=get;watch

func (r *ProcessorReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("processor", req.NamespacedName)

	var original streamingv1alpha1.Processor
	if err := r.Client.Get(ctx, req.NamespacedName, &original); err != nil {
		return ctrl.Result{}, ignoreNotFound(err)
	}

	// Don't modify the informers copy
	processor := original.DeepCopy()

	// Reconcile this copy of the processor and then write back any status
	// updates regardless of whether the reconciliation errored out.
	result, err := r.reconcile(ctx, log, processor)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(original.Status, processor.Status) {
		log.Info("updating processor status", "diff", cmp.Diff(original.Status, processor.Status))
		if updateErr := r.Status().Update(ctx, processor); updateErr != nil {
			log.Error(updateErr, "unable to update Processor status")
			return ctrl.Result{Requeue: true}, updateErr
		}
	}
	return result, err
}

func (r *ProcessorReconciler) reconcile(ctx context.Context, logger logr.Logger, processor *streamingv1alpha1.Processor) (ctrl.Result, error) {
	if processor.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	processor.Default()

	processor.Status.InitializeConditions()

	processorNSName := namespacedNamedFor(processor)

	// Lookup and track configMap to know which images to use
	cm := corev1.ConfigMap{}
	cmKey := types.NamespacedName{Namespace: r.Namespace, Name: processorImages}
	r.Tracker.Track(
		tracker.NewKey(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, cmKey),
		processorNSName,
	)
	if err := r.Get(ctx, cmKey, &cm); err != nil {
		logger.Error(err, "unable to lookup images configMap")
		return ctrl.Result{}, err
	}

	// resolve image
	if processor.Spec.Build != nil {
		if processor.Spec.Build.FunctionRef != "" {
			functionNSName := types.NamespacedName{Namespace: processor.Namespace, Name: processor.Spec.Build.FunctionRef}
			var function v1alpha1.Function
			r.Tracker.Track(
				tracker.NewKey(function.GetGroupVersionKind(), functionNSName),
				processorNSName,
			)
			if err := r.Client.Get(ctx, functionNSName, &function); err != nil {
				if errors.IsNotFound(err) {
					// we'll ignore not-found errors, since the reference build resource may not exist yet.
					return ctrl.Result{}, nil
				}
				return ctrl.Result{Requeue: true}, err
			}

			processor.Status.LatestImage = function.Status.LatestImage

		} else if processor.Spec.Build.ContainerRef != "" {
			containerNSName := types.NamespacedName{Namespace: processor.Namespace, Name: processor.Spec.Build.ContainerRef}
			var container v1alpha1.Container
			r.Tracker.Track(
				tracker.NewKey(container.GetGroupVersionKind(), containerNSName),
				processorNSName,
			)
			if err := r.Client.Get(ctx, containerNSName, &container); err != nil {
				if errors.IsNotFound(err) {
					// we'll ignore not-found errors, since the reference build resource may not exist yet.
					return ctrl.Result{}, nil
				}
				return ctrl.Result{Requeue: true}, err
			}

			processor.Status.LatestImage = container.Status.LatestImage
		}
	} else {
		// defaulter guarantees a container
		processor.Status.LatestImage = processor.Spec.Template.Containers[0].Image
	}

	if processor.Status.LatestImage == "" {
		return ctrl.Result{}, fmt.Errorf("could not resolve an image")
	}

	// Resolve input addresses
	inputStreams, err := r.resolveStreams(ctx, processorNSName, processor.Spec.Inputs)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}
	processor.Status.DeprecatedInputAddresses = r.collectStreamAddresses(inputStreams)

	// Resolve output addresses
	outputStreams, err := r.resolveStreams(ctx, processorNSName, processor.Spec.Outputs)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}
	processor.Status.DeprecatedOutputAddresses = r.collectStreamAddresses(outputStreams)
	processor.Status.DeprecatedOutputContentTypes = r.collectStreamContentTypes(outputStreams)

	// Reconcile deployment for processor
	deployment, err := r.reconcileProcessorDeployment(ctx, logger, processor, inputStreams, outputStreams, &cm)
	if err != nil {
		logger.Error(err, "unable to reconcile deployment")
		return ctrl.Result{}, err
	}
	processor.Status.DeploymentName = deployment.Name
	processor.Status.PropagateDeploymentStatus(&deployment.Status)

	processor.Status.MarkStreamsReady()
	streams := []streamingv1alpha1.Stream{}
	streams = append(streams, inputStreams...)
	streams = append(streams, outputStreams...)
	for _, stream := range streams {
		ready := stream.Status.GetCondition(stream.Status.GetReadyConditionType())
		if ready == nil {
			ready = &apis.Condition{Message: "stream has no ready condition"}
		}
		if !ready.IsTrue() {
			processor.Status.MarkStreamsNotReady(fmt.Sprintf("stream %s is not ready: %s", stream.Name, ready.Message))
			break
		}
	}

	// Reconcile scaledObject for processor
	scaledObject, err := r.reconcileProcessorScaledObject(ctx, logger, processor, deployment)
	if err != nil {
		logger.Error(err, "unable to reconcile scaledObject")
		return ctrl.Result{}, err
	}
	processor.Status.ScaledObjectName = scaledObject.Name
	processor.Status.PropagateScaledObjectStatus(&scaledObject.Status)

	processor.Status.ObservedGeneration = processor.Generation

	return ctrl.Result{}, nil
}

func (r *ProcessorReconciler) reconcileProcessorScaledObject(ctx context.Context, log logr.Logger, processor *streamingv1alpha1.Processor, deployment *appsv1.Deployment) (*kedav1alpha1.ScaledObject, error) {
	var actualScaledObject kedav1alpha1.ScaledObject
	var childScaledObjects kedav1alpha1.ScaledObjectList
	if err := r.List(ctx, &childScaledObjects, client.InNamespace(processor.Namespace), client.MatchingField(processorScaledObjectIndexField, processor.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childScaledObjects.Items) == 1 {
		actualScaledObject = childScaledObjects.Items[0]
	} else if len(childScaledObjects.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraScaledObject := range childScaledObjects.Items {
			log.Info("deleting extra scaled object", "scaledObject", extraScaledObject)
			if err := r.Delete(ctx, &extraScaledObject); err != nil {
				return nil, err
			}
		}
	}

	desiredScaledObject, err := r.constructScaledObjectForProcessor(processor, deployment)
	if err != nil {
		return nil, err
	}

	// delete scaledObject if no longer needed
	if desiredScaledObject == nil {
		if err := r.Delete(ctx, &actualScaledObject); err != nil {
			log.Error(err, "unable to delete ScaledObject for Processor", "scaledObject", actualScaledObject)
			return nil, err
		}
		return nil, nil
	}

	// create scaledObject if it doesn't exist
	if actualScaledObject.Name == "" {
		log.Info("creating scaled object", "spec", desiredScaledObject.Spec)
		if err := r.Create(ctx, desiredScaledObject); err != nil {
			log.Error(err, "unable to create ScaledObject for Processor", "scaledObject", desiredScaledObject)
			return nil, err
		}
		return desiredScaledObject, nil
	}

	if r.scaledObjectSemanticEquals(desiredScaledObject, &actualScaledObject) {
		// scaledObject is unchanged
		return &actualScaledObject, nil
	}

	// update scaledObject with desired changes

	scaledObject := actualScaledObject.DeepCopy()
	scaledObject.ObjectMeta.Labels = desiredScaledObject.ObjectMeta.Labels
	scaledObject.Spec = desiredScaledObject.Spec
	log.Info("reconciling scaled object", "diff", cmp.Diff(actualScaledObject.Spec, scaledObject.Spec))
	if err := r.Update(ctx, scaledObject); err != nil {
		log.Error(err, "unable to update ScaledObject for Processor", "scaledObject", scaledObject)
		return nil, err
	}

	return scaledObject, nil
}

func (r *ProcessorReconciler) constructScaledObjectForProcessor(processor *streamingv1alpha1.Processor, deployment *appsv1.Deployment) (*kedav1alpha1.ScaledObject, error) {
	labels := r.constructLabelsForProcessor(processor)

	zero := int32(0)
	one := int32(1)
	thirty := int32(30)

	labels["deploymentName"] = deployment.Name

	maxReplicas := thirty
	if processor.Status.GetCondition(streamingv1alpha1.ProcessorConditionStreamsReady).IsFalse() {
		// scale to zero while dependencies are not ready
		maxReplicas = zero
	}

	scaledObject := &kedav1alpha1.ScaledObject{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-processor-", processor.Name),
			Namespace:    processor.Namespace,
			Labels:       labels,
		},
		Spec: kedav1alpha1.ScaledObjectSpec{
			ScaleTargetRef: &kedav1alpha1.ObjectReference{
				DeploymentName: deployment.Name,
			},
			PollingInterval: &one,
			CooldownPeriod:  &thirty,
			Triggers:        triggers(processor),
			MinReplicaCount: &zero,
			MaxReplicaCount: &maxReplicas,
		},
	}

	if err := ctrl.SetControllerReference(processor, scaledObject, r.Scheme); err != nil {
		return nil, err
	}

	return scaledObject, nil
}

func triggers(proc *streamingv1alpha1.Processor) []kedav1alpha1.ScaleTriggers {
	result := make([]kedav1alpha1.ScaleTriggers, len(proc.Status.DeprecatedInputAddresses))
	for i, topic := range proc.Status.DeprecatedInputAddresses {
		result[i].Type = "liiklus"
		result[i].Metadata = map[string]string{
			"address": strings.SplitN(topic, "/", 2)[0],
			"group":   proc.Name,
			"topic":   strings.SplitN(topic, "/", 2)[1],
		}
	}
	return result
}

func (r *ProcessorReconciler) scaledObjectSemanticEquals(desiredDeployment, deployment *kedav1alpha1.ScaledObject) bool {
	return equality.Semantic.DeepEqual(desiredDeployment.Spec, deployment.Spec) &&
		equality.Semantic.DeepEqual(desiredDeployment.ObjectMeta.Labels, deployment.ObjectMeta.Labels)
}

func (r *ProcessorReconciler) reconcileProcessorDeployment(ctx context.Context, log logr.Logger, processor *streamingv1alpha1.Processor, inputStreams, outputStreams []streamingv1alpha1.Stream, cm *corev1.ConfigMap) (*appsv1.Deployment, error) {
	var actualDeployment appsv1.Deployment
	var childDeployments appsv1.DeploymentList
	if err := r.List(ctx, &childDeployments, client.InNamespace(processor.Namespace), client.MatchingField(processorDeploymentIndexField, processor.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childDeployments.Items) == 1 {
		actualDeployment = childDeployments.Items[0]
	} else if len(childDeployments.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraDeployment := range childDeployments.Items {
			log.Info("deleting extra deployment", "deployment", extraDeployment)
			if err := r.Delete(ctx, &extraDeployment); err != nil {
				return nil, err
			}
		}
	}

	processorImg := cm.Data[processorImageKey]
	if processorImg == "" {
		return nil, fmt.Errorf("missing processor image configuration")
	}

	desiredDeployment, err := r.constructDeploymentForProcessor(processor, inputStreams, outputStreams, processorImg)
	if err != nil {
		return nil, err
	}

	// delete deployment if no longer needed
	if desiredDeployment == nil {
		if err := r.Delete(ctx, &actualDeployment); err != nil {
			log.Error(err, "unable to delete Deployment for Processor", "deployment", actualDeployment)
			return nil, err
		}
		return nil, nil
	}

	// create deployment if it doesn't exist
	if actualDeployment.Name == "" {
		log.Info("creating processor deployment", "spec", desiredDeployment.Spec)
		if err := r.Create(ctx, desiredDeployment); err != nil {
			log.Error(err, "unable to create Deployment for Processor", "deployment", desiredDeployment)
			return nil, err
		}
		return desiredDeployment, nil
	}

	// overwrite fields that should not be mutated
	desiredDeployment.Spec.Replicas = actualDeployment.Spec.Replicas

	if r.deploymentSemanticEquals(desiredDeployment, &actualDeployment) {
		// deployment is unchanged
		return &actualDeployment, nil
	}

	// update deployment with desired changes

	deployment := actualDeployment.DeepCopy()
	deployment.ObjectMeta.Labels = desiredDeployment.ObjectMeta.Labels
	deployment.Spec = desiredDeployment.Spec
	log.Info("reconciling processor deployment", "diff", cmp.Diff(actualDeployment.Spec, deployment.Spec))
	if err := r.Update(ctx, deployment); err != nil {
		log.Error(err, "unable to update Deployment for Processor", "deployment", deployment)
		return nil, err
	}

	return deployment, nil
}

func (r *ProcessorReconciler) constructDeploymentForProcessor(processor *streamingv1alpha1.Processor, inputStreams, outputStreams []streamingv1alpha1.Stream, processorImg string) (*appsv1.Deployment, error) {
	labels := r.constructLabelsForProcessor(processor)

	zero := int32(0)
	environmentVariables, err := r.computeEnvironmentVariables(processor)
	if err != nil {
		return nil, err
	}

	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	// De-dupe streams and create one volume for each
	streams := make(map[string]streamingv1alpha1.Stream)
	for _, s := range inputStreams {
		streams[s.Name] = s
	}
	for _, s := range outputStreams {
		streams[s.Name] = s
	}
	for _, stream := range streams {
		if stream.Status.Binding.MetadataRef.Name != "" {
			metadataVolumeName := fmt.Sprintf("processor-stream-%s-metadata", stream.Name)
			volumes = append(volumes,
				corev1.Volume{
					Name: metadataVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: stream.Status.Binding.MetadataRef.Name,
							},
						},
					},
				},
			)
		}
		if stream.Status.Binding.SecretRef.Name != "" {
			secretVolumeName := fmt.Sprintf("processor-stream-%s-secret", stream.Name)
			volumes = append(volumes,
				corev1.Volume{
					Name: secretVolumeName,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: stream.Status.Binding.SecretRef.Name,
						},
					},
				},
			)
		}
	}
	// Create one volume mount for each *binding*, split into inputs/outputs.
	// The consumer of those will know to count from 0..Nbindings-1 thanks to the INPUT/OUTPUT_NAMES var
	for i, binding := range processor.Spec.Inputs {
		stream := streams[binding.Stream]
		if stream.Status.Binding.MetadataRef.Name != "" {
			metadataVolumeName := fmt.Sprintf("processor-stream-%s-metadata", stream.Name)
			volumeMounts = append(volumeMounts,
				corev1.VolumeMount{
					Name:      metadataVolumeName,
					MountPath: fmt.Sprintf("%s/input_%03d/metadata", bindingsRootPath, i),
					ReadOnly:  true,
				},
			)
		}
		if stream.Status.Binding.SecretRef.Name != "" {
			secretVolumeName := fmt.Sprintf("processor-stream-%s-secret", stream.Name)
			volumeMounts = append(volumeMounts,
				corev1.VolumeMount{
					Name:      secretVolumeName,
					MountPath: fmt.Sprintf("%s/input_%03d/secret", bindingsRootPath, i),
					ReadOnly:  true,
				},
			)
		}
	}
	for i, binding := range processor.Spec.Outputs {
		stream := streams[binding.Stream]
		if stream.Status.Binding.MetadataRef.Name != "" {
			metadataVolumeName := fmt.Sprintf("processor-stream-%s-metadata", stream.Name)
			volumeMounts = append(volumeMounts,
				corev1.VolumeMount{
					Name:      metadataVolumeName,
					MountPath: fmt.Sprintf("%s/output_%03d/metadata", bindingsRootPath, i),
					ReadOnly:  true,
				},
			)
		}
		if stream.Status.Binding.SecretRef.Name != "" {
			secretVolumeName := fmt.Sprintf("processor-stream-%s-secret", stream.Name)
			volumeMounts = append(volumeMounts,
				corev1.VolumeMount{
					Name:      secretVolumeName,
					MountPath: fmt.Sprintf("%s/output_%03d/secret", bindingsRootPath, i),
					ReadOnly:  true,
				},
			)
		}
	}

	// merge provided template with controlled values
	podSpec := processor.Spec.Template.DeepCopy()
	podSpec.Containers[0].Image = processor.Status.LatestImage
	podSpec.Containers[0].Ports = []v1.ContainerPort{
		{
			ContainerPort: 8081,
		},
	}
	podSpec.Containers = append(podSpec.Containers, v1.Container{
		Name:            "processor",
		Image:           processorImg,
		ImagePullPolicy: v1.PullIfNotPresent,
		Env:             environmentVariables,
		VolumeMounts:    volumeMounts,
	})
	podSpec.Volumes = append(podSpec.Volumes, volumes...)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-processor-", processor.Name),
			Namespace:    processor.Namespace,
			Labels:       labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &zero,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					streamingv1alpha1.ProcessorLabelKey: processor.Name,
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						streamingv1alpha1.ProcessorLabelKey: processor.Name,
					},
				},
				Spec: *podSpec,
			},
		},
	}
	if err := ctrl.SetControllerReference(processor, deployment, r.Scheme); err != nil {
		return nil, err
	}

	return deployment, nil
}

func (r *ProcessorReconciler) constructLabelsForProcessor(processor *streamingv1alpha1.Processor) map[string]string {
	labels := make(map[string]string, len(processor.ObjectMeta.Labels)+1)
	// pass through existing labels
	for k, v := range processor.ObjectMeta.Labels {
		labels[k] = v
	}

	labels[streamingv1alpha1.ProcessorLabelKey] = processor.Name
	return labels
}

func (r *ProcessorReconciler) deploymentSemanticEquals(desiredDeployment, deployment *appsv1.Deployment) bool {
	return equality.Semantic.DeepEqual(desiredDeployment.Spec, deployment.Spec) &&
		equality.Semantic.DeepEqual(desiredDeployment.ObjectMeta.Labels, deployment.ObjectMeta.Labels)
}

func (r *ProcessorReconciler) resolveStreams(ctx context.Context, processorCoordinates types.NamespacedName, bindings []streamingv1alpha1.StreamBinding) ([]streamingv1alpha1.Stream, error) {
	streams := make([]streamingv1alpha1.Stream, len(bindings))
	for i, binding := range bindings {
		streamNSName := types.NamespacedName{
			Namespace: processorCoordinates.Namespace,
			Name:      binding.Stream,
		}
		var stream streamingv1alpha1.Stream
		// track stream for new coordinates
		r.Tracker.Track(
			tracker.NewKey(stream.GetGroupVersionKind(), streamNSName),
			processorCoordinates,
		)
		if err := r.Client.Get(ctx, streamNSName, &stream); err != nil {
			return nil, err
		}
		streams[i] = stream
	}
	return streams, nil
}

func (r *ProcessorReconciler) collectStreamAddresses(streams []streamingv1alpha1.Stream) []string {
	addresses := make([]string, len(streams))
	for i, stream := range streams {
		addresses[i] = stream.Status.Address.String()
	}
	return addresses
}

func (r *ProcessorReconciler) collectStreamContentTypes(streams []streamingv1alpha1.Stream) []string {
	contentTypes := make([]string, len(streams))
	for i, stream := range streams {
		contentTypes[i] = stream.Spec.ContentType
	}
	return contentTypes
}

func (r *ProcessorReconciler) computeEnvironmentVariables(processor *streamingv1alpha1.Processor) ([]v1.EnvVar, error) {
	contentTypesJson, err := json.Marshal(processor.Status.DeprecatedOutputContentTypes)
	if err != nil {
		return nil, err
	}
	inputsNames := r.collectAliases(processor.Spec.Inputs)
	outputsNames := r.collectAliases(processor.Spec.Outputs)
	return []v1.EnvVar{
		{
			Name:  "CNB_BINDINGS",
			Value: bindingsRootPath,
		},
		{
			// TODO remove once the processor images consumes bindings
			Name:  "INPUTS",
			Value: strings.Join(processor.Status.DeprecatedInputAddresses, ","),
		},
		{
			// TODO remove once the processor images consumes bindings
			Name:  "OUTPUTS",
			Value: strings.Join(processor.Status.DeprecatedOutputAddresses, ","),
		},
		{
			Name:  "INPUT_NAMES",
			Value: strings.Join(inputsNames, ","),
		},
		{
			Name:  "OUTPUT_NAMES",
			Value: strings.Join(outputsNames, ","),
		},
		{
			Name:  "GROUP",
			Value: processor.Name,
		},
		{
			Name:  "FUNCTION",
			Value: "localhost:8081",
		},
		{
			// TODO remove once the processor images consumes bindings
			Name:  "OUTPUT_CONTENT_TYPES",
			Value: string(contentTypesJson),
		},
	}, nil
}

func (*ProcessorReconciler) collectAliases(bindings []streamingv1alpha1.StreamBinding) []string {
	names := make([]string, len(bindings))
	for i := range bindings {
		names[i] = bindings[i].Alias
	}
	return names
}

func (r *ProcessorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	enqueueTrackedResources := func(t apis.Resource) handler.EventHandler {
		return &handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
				requests := []reconcile.Request{}
				key := tracker.NewKey(
					t.GetGroupVersionKind(),
					types.NamespacedName{Namespace: a.Meta.GetNamespace(), Name: a.Meta.GetName()},
				)
				for _, item := range r.Tracker.Lookup(key) {
					requests = append(requests, reconcile.Request{NamespacedName: item})
				}
				return requests
			}),
		}
	}

	if err := controllers.IndexControllersOfType(mgr, processorDeploymentIndexField, &streamingv1alpha1.Processor{}, &appsv1.Deployment{}); err != nil {
		return err
	}
	if err := controllers.IndexControllersOfType(mgr, processorScaledObjectIndexField, &streamingv1alpha1.Processor{}, &kedav1alpha1.ScaledObject{}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&streamingv1alpha1.Processor{}).
		Owns(&appsv1.Deployment{}).
		Owns(&kedav1alpha1.ScaledObject{}).
		Watches(&source.Kind{Type: &buildv1alpha1.Container{}}, enqueueTrackedResources(&buildv1alpha1.Container{})).
		Watches(&source.Kind{Type: &buildv1alpha1.Function{}}, enqueueTrackedResources(&buildv1alpha1.Function{})).
		Watches(&source.Kind{Type: &streamingv1alpha1.Stream{}}, enqueueTrackedResources(&streamingv1alpha1.Stream{})).
		Complete(r)
}
