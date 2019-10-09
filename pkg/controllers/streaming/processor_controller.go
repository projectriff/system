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
	apierrs "k8s.io/apimachinery/pkg/api/errors"
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

	"github.com/projectriff/system/pkg/apis/build/v1alpha1"
	"github.com/projectriff/system/pkg/tracker"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
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
// +kubebuilder:rbac:groups=build.projectriff.io,resources=functions,verbs=get;watch

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
	processor.SetDefaults(ctx)

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

	// resolve function
	functionNSName := types.NamespacedName{Namespace: processor.Namespace, Name: processor.Spec.FunctionRef}
	var function v1alpha1.Function
	r.Tracker.Track(
		tracker.NewKey(function.GetGroupVersionKind(), functionNSName),
		processorNSName,
	)
	if err := r.Client.Get(ctx, functionNSName, &function); err != nil {
		if errors.IsNotFound(err) {
			processor.Status.MarkFunctionNotFound(processor.Spec.FunctionRef)
		}
		return ctrl.Result{Requeue: true}, err
	}

	processor.Status.PropagateFunctionStatus(&function.Status)
	if processor.Status.FunctionImage == "" {
		return ctrl.Result{}, fmt.Errorf("function %q does not have a latestImage", function.Name)
	}

	// Resolve input addresses
	inputAddresses, _, err := r.resolveStreams(ctx, processorNSName, processor.Spec.Inputs)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}
	processor.Status.InputAddresses = inputAddresses

	// Resolve output addresses
	outputAddresses, outputContentTypes, err := r.resolveStreams(ctx, processorNSName, processor.Spec.Outputs)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}
	processor.Status.OutputAddresses = outputAddresses
	processor.Status.OutputContentTypes = outputContentTypes

	// Reconcile deployment for processor
	deployment, err := r.reconcileProcessorDeployment(ctx, logger, processor, &cm)
	if err != nil {
		logger.Error(err, "unable to reconcile deployment")
		return ctrl.Result{}, err
	}
	processor.Status.DeploymentName = deployment.Name
	processor.Status.PropagateDeploymentStatus(&deployment.Status)

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
	if processor.Status.ScaledObjectName != "" {
		namespacedName := types.NamespacedName{Namespace: processor.Namespace, Name: processor.Status.ScaledObjectName}
		if err := r.Get(ctx, namespacedName, &actualScaledObject); err != nil {
			log.Error(err, "unable to fetch ScaledObject")
			if !apierrs.IsNotFound(err) {
				return nil, err
			}
			// reset the ScaledObjectName since it no longer exists and needs to
			// be recreated
			processor.Status.ScaledObjectName = ""
		}
		// check that the scaledObject is not controlled by another resource
		if !metav1.IsControlledBy(&actualScaledObject, processor) {
			processor.Status.MarkScaledObjectNotOwned()
			return nil, fmt.Errorf("processor %q does not own ScaledObject %q", processor.Name, actualScaledObject.Name)
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
	if processor.Status.ScaledObjectName == "" {
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

	scaledObject := &kedav1alpha1.ScaledObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scaledObjectName(processor),
			Namespace: processor.Namespace,
			Labels:    labels,
		},
		Spec: kedav1alpha1.ScaledObjectSpec{
			ScaleTargetRef: kedav1alpha1.ObjectReference{
				DeploymentName: deployment.Name,
			},
			PollingInterval: &one,
			CooldownPeriod:  &thirty,
			Triggers:        triggers(processor),
			MinReplicaCount: &zero,
			MaxReplicaCount: &thirty,
		},
	}

	if err := ctrl.SetControllerReference(processor, scaledObject, r.Scheme); err != nil {
		return nil, err
	}

	return scaledObject, nil
}

func triggers(proc *streamingv1alpha1.Processor) []kedav1alpha1.ScaleTriggers {
	result := make([]kedav1alpha1.ScaleTriggers, len(proc.Status.InputAddresses))
	for i, topic := range proc.Status.InputAddresses {
		result[i].Type = "liiklus"
		result[i].Metadata = map[string]string{
			"address": strings.Split(topic, "/")[0],
			"group":   proc.Name,
			"topic":   strings.Split(topic, "/")[1],
		}
	}
	return result
}

func (r *ProcessorReconciler) scaledObjectSemanticEquals(desiredDeployment, deployment *kedav1alpha1.ScaledObject) bool {
	return equality.Semantic.DeepEqual(desiredDeployment.Spec, deployment.Spec) &&
		equality.Semantic.DeepEqual(desiredDeployment.ObjectMeta.Labels, deployment.ObjectMeta.Labels)
}

func (r *ProcessorReconciler) reconcileProcessorDeployment(ctx context.Context, log logr.Logger, processor *streamingv1alpha1.Processor, cm *corev1.ConfigMap) (*appsv1.Deployment, error) {
	var actualDeployment appsv1.Deployment
	if processor.Status.DeploymentName != "" {
		namespacedName := types.NamespacedName{Namespace: processor.Namespace, Name: processor.Status.DeploymentName}
		if err := r.Get(ctx, namespacedName, &actualDeployment); err != nil {
			log.Error(err, "unable to fetch Deployment")
			if !apierrs.IsNotFound(err) {
				return nil, err
			}
			// reset the DeploymentName since it no longer exists and needs to
			// be recreated
			processor.Status.DeploymentName = ""
		}
		// check that the deployment is not controlled by another resource
		if !metav1.IsControlledBy(&actualDeployment, processor) {
			processor.Status.MarkDeploymentNotOwned()
			return nil, fmt.Errorf("processor %q does not own Deployment %q", processor.Name, actualDeployment.Name)
		}
	}

	processorImg := cm.Data[processorImageKey]
	if processorImg == "" {
		return nil, fmt.Errorf("missing processor image configuration")
	}

	desiredDeployment, err := r.constructDeploymentForProcessor(processor, processorImg)
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
	if processor.Status.DeploymentName == "" {
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

func (r *ProcessorReconciler) constructDeploymentForProcessor(processor *streamingv1alpha1.Processor, processorImg string) (*appsv1.Deployment, error) {
	labels := r.constructLabelsForProcessor(processor)

	one := int32(1)
	environmentVariables, err := r.computeEnvironmentVariables(processor)
	if err != nil {
		return nil, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName(processor),
			Namespace: processor.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &one,
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
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:            "processor",
							Image:           processorImg,
							ImagePullPolicy: v1.PullAlways,
							Env:             environmentVariables,
						},
						{
							Name:  "function",
							Image: processor.Status.FunctionImage,
							Ports: []v1.ContainerPort{
								{
									ContainerPort: 8081,
								},
							},
						},
					},
				},
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

func (r *ProcessorReconciler) resolveStreams(ctx context.Context, processorCoordinates types.NamespacedName, streamNames []string) ([]string, []string, error) {
	var addresses []string
	var contentTypes []string
	for _, streamName := range streamNames {
		streamNSName := types.NamespacedName{
			Namespace: processorCoordinates.Namespace,
			Name:      streamName,
		}
		var stream streamingv1alpha1.Stream
		// track stream for new cordinates
		r.Tracker.Track(
			tracker.NewKey(stream.GetGroupVersionKind(), streamNSName),
			processorCoordinates,
		)
		if err := r.Client.Get(ctx, streamNSName, &stream); err != nil {
			return nil, nil, err
		}
		addresses = append(addresses, stream.Status.Address.String())
		contentTypes = append(contentTypes, stream.Spec.ContentType)
	}
	return addresses, contentTypes, nil
}

func (r *ProcessorReconciler) computeEnvironmentVariables(processor *streamingv1alpha1.Processor) ([]v1.EnvVar, error) {
	contentTypesJson, err := r.serializeContentTypes(processor.Status.OutputContentTypes)
	if err != nil {
		return nil, err
	}
	return []v1.EnvVar{
		{
			Name:  "INPUTS",
			Value: strings.Join(processor.Status.InputAddresses, ","),
		},
		{
			Name:  "OUTPUTS",
			Value: strings.Join(processor.Status.OutputAddresses, ","),
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
			Name:  "OUTPUT_CONTENT_TYPES",
			Value: contentTypesJson,
		},
	}, nil
}

func deploymentName(p *streamingv1alpha1.Processor) string {
	return fmt.Sprintf("%s-processor", p.Name)
}

func scaledObjectName(p *streamingv1alpha1.Processor) string {
	return fmt.Sprintf("%s-processor", p.Name)
}

type outputContentType struct {
	OutputIndex int    `json:"outputIndex"`
	ContentType string `json:"contentType"`
}

func (r *ProcessorReconciler) serializeContentTypes(outputContentTypes []string) (string, error) {
	outputCount := len(outputContentTypes)
	result := make([]outputContentType, outputCount)
	for i := 0; i < outputCount; i++ {
		result[i] = outputContentType{
			OutputIndex: i,
			ContentType: outputContentTypes[i],
		}
	}
	bytes, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&streamingv1alpha1.Processor{}).
		Owns(&appsv1.Deployment{}).
		Owns(&kedav1alpha1.ScaledObject{}).
		Watches(&source.Kind{Type: &buildv1alpha1.Function{}}, enqueueTrackedResources(&buildv1alpha1.Function{})).
		Watches(&source.Kind{Type: &streamingv1alpha1.Stream{}}, enqueueTrackedResources(&streamingv1alpha1.Stream{})).
		Complete(r)
}
