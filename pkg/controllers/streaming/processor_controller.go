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
	"github.com/projectriff/system/pkg/apis/build/v1alpha1"
	"github.com/projectriff/system/pkg/apis/streaming/v1alpha1/resources"
	"github.com/projectriff/system/pkg/apis/streaming/v1alpha1/resources/names"
	"github.com/projectriff/system/pkg/controllers/kmp"
	"github.com/projectriff/system/pkg/tracker"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
)

// ProcessorReconciler reconciles a Processor object
type ProcessorReconciler struct {
	client.Client
	Log     logr.Logger
	Scheme  *runtime.Scheme
	Tracker tracker.Tracker
}

// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=processors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=processors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

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
		if updateErr := r.Status().Update(ctx, processor); updateErr != nil {
			log.Error(updateErr, "unable to update Processor status", "processor", processor)
			return ctrl.Result{Requeue: true}, updateErr
		}
	}
	return result, err
}

func (r *ProcessorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	enqueueTrackedResources := &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
			requests := []reconcile.Request{}
			for _, item := range r.Tracker.Lookup(a.Object.(metav1.ObjectMetaAccessor)) {
				requests = append(requests, reconcile.Request{NamespacedName: item})
			}
			return requests
		}),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&streamingv1alpha1.Processor{}).
		Owns(&appsv1.Deployment{}).
		Watches(&source.Kind{Type: &buildv1alpha1.Function{}}, enqueueTrackedResources).
		Complete(r)
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

	// resolve function
	functionName := types.NamespacedName{
		Namespace: processor.Namespace,
		Name:      processor.Spec.FunctionRef,
	}
	var function v1alpha1.Function
	if err := r.Client.Get(ctx, functionName, &function); err != nil {
		return ctrl.Result{Requeue: true}, ignoreNotFound(err)
	}

	processorName := types.NamespacedName{
		Namespace: processor.GetNamespace(),
		Name:      processor.GetName(),
	}
	if err := r.Tracker.Track(&function, processorName); err != nil {
		return ctrl.Result{Requeue: true}, err
	}
	processor.Status.PropagateFunctionStatus(&function.Status)

	inputAddresses, _, err := r.resolveStreams(ctx, processorName, processor.Spec.Inputs)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}
	processor.Status.InputAddresses = inputAddresses
	outputAddresses, outputContentTypes, err := r.resolveStreams(ctx, processorName, processor.Spec.Outputs)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}
	processor.Status.OutputAddresses = outputAddresses
	processor.Status.OutputContentTypes = outputContentTypes

	deploymentName := types.NamespacedName{
		Namespace: processor.GetNamespace(),
		Name:      names.Deployment(processor),
	}
	var deployment *appsv1.Deployment
	err = r.Client.Get(ctx, deploymentName, deployment)
	if errors.IsNotFound(err) {
		if deployment, err = r.createDeployment(ctx, processor); err != nil {
			logger.Error(err, "Failed to create Deployment %q: %v", deploymentName, err)
			return ctrl.Result{Requeue: true}, err
		}
	} else if err != nil {
		logger.Error(err, "Failed to reconcile Processor: %q failed to Get Deployment: %q; %v", processor.Name, deploymentName, zap.Error(err))
		return ctrl.Result{Requeue: true}, err
	} else if !metav1.IsControlledBy(deployment, processor) {
		// Surface an error in the processor's status,and return an error.
		processor.Status.MarkDeploymentNotOwned(deploymentName.Name)
		return ctrl.Result{}, fmt.Errorf("Processor: %q does not own Deployment: %q", processor.Name, deploymentName)
	} else {
		if deployment, err = r.reconcileDeployment(ctx, logger, processor, deployment); err != nil {
			logger.Error(err, "Failed to reconcile Processor: %q failed to reconcile Deployment: %q; %v", processor.Name, deployment, zap.Error(err))
			return ctrl.Result{Requeue: true}, err
		}
	}

	// Update our Status based on the state of our underlying Deployment.
	processor.Status.DeploymentName = deployment.Name
	processor.Status.PropagateDeploymentStatus(&deployment.Status)
	processor.Status.ObservedGeneration = processor.Generation

	return ctrl.Result{}, nil
}

func (r *ProcessorReconciler) reconcileDeployment(ctx context.Context, logger logr.Logger, processor *streamingv1alpha1.Processor, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
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
	logger.Info("Reconciling deployment diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := deployment.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredDeployment.Spec
	existing.ObjectMeta.Labels = desiredDeployment.ObjectMeta.Labels

	return existing, r.Client.Update(ctx, existing)
}

func (r *ProcessorReconciler) createDeployment(ctx context.Context, processor *streamingv1alpha1.Processor) (*appsv1.Deployment, error) {
	deployment, err := resources.MakeDeployment(processor)
	if err != nil {
		return nil, err
	}
	if deployment == nil {
		// nothing to create
		return deployment, nil
	}
	return deployment, r.Client.Create(ctx, deployment)
}

func deploymentSemanticEquals(desiredDeployment, deployment *appsv1.Deployment) bool {
	return equality.Semantic.DeepEqual(desiredDeployment.Spec, deployment.Spec) &&
		equality.Semantic.DeepEqual(desiredDeployment.ObjectMeta.Labels, deployment.ObjectMeta.Labels)
}

func (r *ProcessorReconciler) resolveStreams(ctx context.Context, processorCoordinates types.NamespacedName, streamNames []string) ([]string, []string, error) {
	var addresses []string
	var contentTypes []string
	for _, streamName := range streamNames {
		streamName := types.NamespacedName{
			Namespace: processorCoordinates.Namespace,
			Name:      streamName,
		}
		var stream streamingv1alpha1.Stream
		if err := r.Client.Get(ctx, streamName, &stream); err != nil {
			return nil, nil, err
		}
		if err := r.Tracker.Track(&stream, processorCoordinates); err != nil {
			return nil, nil, err
		}
		addresses = append(addresses, stream.Status.Address.String())
		contentTypes = append(contentTypes, stream.Spec.ContentType)
	}
	return addresses, contentTypes, nil
}
