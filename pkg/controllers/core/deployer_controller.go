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

package core

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	corev1alpha1 "github.com/projectriff/system/pkg/apis/core/v1alpha1"
	"github.com/projectriff/system/pkg/tracker"
)

// DeployerReconciler reconciles a Deployer object
type DeployerReconciler struct {
	client.Client
	Log     logr.Logger
	Scheme  *runtime.Scheme
	Tracker tracker.Tracker
}

// +kubebuilder:rbac:groups=core.projectriff.io,resources=deployers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.projectriff.io,resources=deployers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=build.projectriff.io,resources=applications;containers;functions,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *DeployerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("deployer", req.NamespacedName)

	var originalDeployer corev1alpha1.Deployer
	if err := r.Get(ctx, req.NamespacedName, &originalDeployer); err != nil {
		if apierrs.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Deployer")
		return ctrl.Result{}, err
	}
	deployer := *(originalDeployer.DeepCopy())

	deployer.SetDefaults(ctx)
	deployer.Status.InitializeConditions()

	result, err := r.reconcile(ctx, log, &deployer)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(deployer.Status, originalDeployer.Status) {
		// update status
		if updateErr := r.Status().Update(ctx, &deployer); updateErr != nil {
			log.Error(updateErr, "unable to update Deployer status", "deployer", deployer)
			return ctrl.Result{Requeue: true}, updateErr
		}
	}

	// return original reconcile result
	return result, err
}

func (r *DeployerReconciler) reconcile(ctx context.Context, log logr.Logger, deployer *corev1alpha1.Deployer) (ctrl.Result, error) {
	if deployer.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	// resolve build image
	if err := r.reconcileBuildImage(ctx, log, deployer); err != nil {
		if apierrs.IsNotFound(err) {
			// we'll ignore not-found errors, since the reference build resource
			// may not exist yet.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to resolve image for Deployer", "deployer", deployer)
		return ctrl.Result{Requeue: true}, err
	}

	// reconcile deployment
	childDeployment, err := r.reconcileChildDeployment(ctx, log, deployer)
	if err != nil {
		log.Error(err, "unable to reconcile child Deployment", "deployer", deployer)
		return ctrl.Result{}, err
	}
	deployer.Status.DeploymentName = childDeployment.Name
	deployer.Status.PropagateDeploymentStatus(&childDeployment.Status)

	// reconcile service
	childService, err := r.reconcileChildService(ctx, log, deployer)
	if err != nil {
		log.Error(err, "unable to reconcile child Service", "deployer", deployer)
		return ctrl.Result{}, err
	}
	deployer.Status.ServiceName = childService.Name
	deployer.Status.PropagateServiceStatus(&childService.Status)

	deployer.Status.ObservedGeneration = deployer.Generation

	return ctrl.Result{}, nil
}

func (r *DeployerReconciler) reconcileBuildImage(ctx context.Context, log logr.Logger, deployer *corev1alpha1.Deployer) error {
	build := deployer.Spec.Build
	if build == nil {
		return nil
	}

	switch {
	case build.ApplicationRef != "":
		var application buildv1alpha1.Application
		key := types.NamespacedName{Namespace: deployer.Namespace, Name: build.ApplicationRef}
		// track application for new images
		r.Tracker.Track(
			tracker.NewKey(application.GetGroupVersionKind(), key),
			types.NamespacedName{Namespace: deployer.Namespace, Name: deployer.Name},
		)
		if err := r.Get(ctx, key, &application); err != nil {
			return err
		}
		if application.Status.LatestImage == "" {
			return fmt.Errorf("application %q does not have a ready image", build.ApplicationRef)
		}
		deployer.Spec.Template.Containers[0].Image = application.Status.LatestImage
		return nil

	case build.ContainerRef != "":
		var container buildv1alpha1.Container
		key := types.NamespacedName{Namespace: deployer.Namespace, Name: build.ContainerRef}
		// track container for new images
		r.Tracker.Track(
			tracker.NewKey(container.GetGroupVersionKind(), key),
			types.NamespacedName{Namespace: deployer.Namespace, Name: deployer.Name},
		)
		if err := r.Get(ctx, key, &container); err != nil {
			return err
		}
		if container.Status.LatestImage == "" {
			return fmt.Errorf("container %q does not have a ready image", build.ContainerRef)
		}
		deployer.Spec.Template.Containers[0].Image = container.Status.LatestImage
		return nil

	case build.FunctionRef != "":
		var function buildv1alpha1.Function
		key := types.NamespacedName{Namespace: deployer.Namespace, Name: build.FunctionRef}
		// track function for new images
		r.Tracker.Track(
			tracker.NewKey(function.GetGroupVersionKind(), key),
			types.NamespacedName{Namespace: deployer.Namespace, Name: deployer.Name},
		)
		if err := r.Get(ctx, key, &function); err != nil {
			return err
		}
		if function.Status.LatestImage == "" {
			return fmt.Errorf("function %q does not have a ready image", build.FunctionRef)
		}
		deployer.Spec.Template.Containers[0].Image = function.Status.LatestImage
		return nil

	}

	return fmt.Errorf("invalid deployer build")
}

func (r *DeployerReconciler) reconcileChildDeployment(ctx context.Context, log logr.Logger, deployer *corev1alpha1.Deployer) (*appsv1.Deployment, error) {
	var actualDeployment appsv1.Deployment
	if deployer.Status.DeploymentName != "" {
		if err := r.Get(ctx, types.NamespacedName{Namespace: deployer.Namespace, Name: deployer.Status.DeploymentName}, &actualDeployment); err != nil {
			log.Error(err, "unable to fetch child Deployment")
			if !apierrs.IsNotFound(err) {
				return nil, err
			}
			// reset the DeploymentName since it no longer exists and needs to
			// be recreated
			deployer.Status.DeploymentName = ""
		}
		// check that the deployment is not controlled by another resource
		if !metav1.IsControlledBy(&actualDeployment, deployer) {
			deployer.Status.MarkDeploymentNotOwned()
			return nil, fmt.Errorf("Deployer %q does not own Deployment %q", deployer.Name, actualDeployment.Name)
		}
	}

	desiredDeployment, err := r.constructDeploymentForDeployer(deployer)
	if err != nil {
		return nil, err
	}

	// delete deployment if no longer needed
	if desiredDeployment == nil {
		if err := r.Delete(ctx, &actualDeployment); err != nil {
			log.Error(err, "unable to delete Deployment for Deployer", "deployment", actualDeployment)
			return nil, err
		}
		return nil, nil
	}

	// create deployment if it doesn't exist
	if deployer.Status.DeploymentName == "" {
		if err := r.Create(ctx, desiredDeployment); err != nil {
			log.Error(err, "unable to create Deployment for Deployer", "deployment", desiredDeployment)
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
	if err := r.Update(ctx, deployment); err != nil {
		log.Error(err, "unable to update Deployment for Deployer", "deployment", deployment)
		return nil, err
	}

	return deployment, nil
}

func (r *DeployerReconciler) deploymentSemanticEquals(desiredDeployment, deployment *appsv1.Deployment) bool {
	return equality.Semantic.DeepEqual(desiredDeployment.Spec, deployment.Spec) &&
		equality.Semantic.DeepEqual(desiredDeployment.ObjectMeta.Labels, deployment.ObjectMeta.Labels)
}

func (r *DeployerReconciler) constructDeploymentForDeployer(deployer *corev1alpha1.Deployer) (*appsv1.Deployment, error) {
	labels := r.constructLabelsForDeployer(deployer)
	podSpec := r.constructPodSpecForDeployer(deployer)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: make(map[string]string),
			// GenerateName: fmt.Sprintf("%s-deployer-", deployer.Name),
			Name:      fmt.Sprintf("%s-deployer", deployer.Name),
			Namespace: deployer.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					corev1alpha1.DeployerLabelKey: deployer.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: podSpec,
			},
		},
	}
	if err := ctrl.SetControllerReference(deployer, deployment, r.Scheme); err != nil {
		return nil, err
	}

	return deployment, nil
}

func (r *DeployerReconciler) constructPodSpecForDeployer(deployer *corev1alpha1.Deployer) corev1.PodSpec {
	podSpec := *deployer.Spec.Template.DeepCopy()

	if podSpec.Containers[0].LivenessProbe == nil {
		podSpec.Containers[0].LivenessProbe = &corev1.Probe{
			Handler: corev1.Handler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(8080),
				},
			},
		}
	}
	if podSpec.Containers[0].ReadinessProbe == nil {
		podSpec.Containers[0].ReadinessProbe = &corev1.Probe{
			Handler: corev1.Handler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(8080),
				},
			},
		}
	}

	return podSpec
}

func (r *DeployerReconciler) reconcileChildService(ctx context.Context, log logr.Logger, deployer *corev1alpha1.Deployer) (*corev1.Service, error) {
	var actualService corev1.Service
	if deployer.Status.ServiceName != "" {
		if err := r.Get(ctx, types.NamespacedName{Namespace: deployer.Namespace, Name: deployer.Status.ServiceName}, &actualService); err != nil {
			log.Error(err, "unable to fetch child Service")
			if !apierrs.IsNotFound(err) {
				return nil, err
			}
			// reset the ServiceName since it no longer exists and needs to
			// be recreated
			deployer.Status.ServiceName = ""
		}
		// check that the service is not controlled by another resource
		if !metav1.IsControlledBy(&actualService, deployer) {
			deployer.Status.MarkServiceNotOwned()
			return nil, fmt.Errorf("Deployer %q does not own Service %q", deployer.Name, actualService.Name)
		}
	}

	desiredService, err := r.constructServiceForDeployer(deployer)
	if err != nil {
		return nil, err
	}

	// delete service if no longer needed
	if desiredService == nil {
		if err := r.Delete(ctx, &actualService); err != nil {
			log.Error(err, "unable to delete Service for Deployer", "service", actualService)
			return nil, err
		}
		return nil, nil
	}

	// create service if it doesn't exist
	if deployer.Status.ServiceName == "" {
		if err := r.Create(ctx, desiredService); err != nil {
			log.Error(err, "unable to create Service for Deployer", "service", desiredService)
			return nil, err
		}
		return desiredService, nil
	}

	// overwrite fields that should not be mutated
	desiredService.Spec.ClusterIP = actualService.Spec.ClusterIP

	if r.serviceSemanticEquals(desiredService, &actualService) {
		// service is unchanged
		return &actualService, nil
	}

	// update service with desired changes
	service := actualService.DeepCopy()
	service.ObjectMeta.Labels = desiredService.ObjectMeta.Labels
	service.Spec = desiredService.Spec
	if err := r.Update(ctx, service); err != nil {
		log.Error(err, "unable to update Service for Deployer", "deployment", service)
		return nil, err
	}

	return service, nil
}

func (r *DeployerReconciler) serviceSemanticEquals(desiredService, service *corev1.Service) bool {
	return equality.Semantic.DeepEqual(desiredService.Spec, service.Spec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels)
}

func (r *DeployerReconciler) constructServiceForDeployer(deployer *corev1alpha1.Deployer) (*corev1.Service, error) {
	labels := r.constructLabelsForDeployer(deployer)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: make(map[string]string),
			// GenerateName: fmt.Sprintf("%s-deployer-", deployer.Name),
			Name:      fmt.Sprintf("%s-deployer", deployer.Name),
			Namespace: deployer.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
			},
			Selector: map[string]string{
				corev1alpha1.DeployerLabelKey: deployer.Name,
			},
		},
	}
	if err := ctrl.SetControllerReference(deployer, service, r.Scheme); err != nil {
		return nil, err
	}

	return service, nil
}

func (r *DeployerReconciler) constructLabelsForDeployer(deployer *corev1alpha1.Deployer) map[string]string {
	labels := make(map[string]string, len(deployer.ObjectMeta.Labels)+1)
	// pass through existing labels
	for k, v := range deployer.ObjectMeta.Labels {
		labels[k] = v
	}

	labels[corev1alpha1.DeployerLabelKey] = deployer.Name

	return labels
}

func (r *DeployerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	enqueueTrackedResources := &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
			requests := []reconcile.Request{}
			key := tracker.NewKey(
				a.Object.GetObjectKind().GroupVersionKind(),
				types.NamespacedName{Namespace: a.Meta.GetNamespace(), Name: a.Meta.GetName()},
			)
			for _, item := range r.Tracker.Lookup(key) {
				requests = append(requests, reconcile.Request{NamespacedName: item})
			}
			return requests
		}),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.Deployer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		// watch for build mutations to update dependent deployers
		Watches(&source.Kind{Type: &buildv1alpha1.Application{}}, enqueueTrackedResources).
		Watches(&source.Kind{Type: &buildv1alpha1.Container{}}, enqueueTrackedResources).
		Watches(&source.Kind{Type: &buildv1alpha1.Function{}}, enqueueTrackedResources).
		Complete(r)
}
