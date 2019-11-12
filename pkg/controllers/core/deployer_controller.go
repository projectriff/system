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
	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
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

	"github.com/projectriff/system/pkg/apis"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	corev1alpha1 "github.com/projectriff/system/pkg/apis/core/v1alpha1"
	"github.com/projectriff/system/pkg/controllers"
	"github.com/projectriff/system/pkg/tracker"
)

const (
	deploymentIndexField = ".metadata.deploymentController"
	serviceIndexField    = ".metadata.serviceController"
	ingressIndexField    = ".metadata.ingressController"

	domain = "example.com"
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
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

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

	deployer.Default()
	deployer.Status.InitializeConditions()

	result, err := r.reconcile(ctx, log, &deployer)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(deployer.Status, originalDeployer.Status) {
		// update status
		log.Info("updating deployer status", "diff", cmp.Diff(originalDeployer.Status, deployer.Status))
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
	deployer.Status.Address = &apis.Addressable{URL: fmt.Sprintf("http://%s.%s.%s", childService.Name, childService.Namespace, "svc.cluster.local")}
	deployer.Status.PropagateServiceStatus(&childService.Status)

	// reconcile ingress
	childIngress, err := r.reconcileIngress(ctx, log, deployer, childService.Name)
	if err != nil {
		log.Error(err, "unable to reconcile Ingress", "deployer", deployer)
		return ctrl.Result{}, err
	}
	if childIngress == nil {
		deployer.Status.IngressName = ""
		deployer.Status.URL = ""
		deployer.Status.MarkIngressNotRequired()
	} else {
		deployer.Status.IngressName = childIngress.Name
		deployer.Status.URL = fmt.Sprintf("http://%s", childIngress.Spec.Rules[0].Host)
		deployer.Status.PropagateIngressStatus(&childIngress.Status)
	}

	deployer.Status.ObservedGeneration = deployer.Generation
	return ctrl.Result{}, nil
}

func (r *DeployerReconciler) reconcileBuildImage(ctx context.Context, log logr.Logger, deployer *corev1alpha1.Deployer) error {
	build := deployer.Spec.Build
	if build == nil {
		deployer.Status.LatestImage = deployer.Spec.Template.Containers[0].Image
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
		deployer.Status.LatestImage = application.Status.LatestImage
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
		deployer.Status.LatestImage = container.Status.LatestImage
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
		deployer.Status.LatestImage = function.Status.LatestImage
		return nil

	}

	return fmt.Errorf("invalid deployer build")
}

func (r *DeployerReconciler) reconcileChildDeployment(ctx context.Context, log logr.Logger, deployer *corev1alpha1.Deployer) (*appsv1.Deployment, error) {
	var actualDeployment appsv1.Deployment
	var childDeployments appsv1.DeploymentList
	if err := r.List(ctx, &childDeployments, client.InNamespace(deployer.Namespace), client.MatchingField(deploymentIndexField, deployer.Name)); err != nil {
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

	desiredDeployment, err := r.constructDeploymentForDeployer(deployer)
	if err != nil {
		return nil, err
	}

	// delete deployment if no longer needed
	if desiredDeployment == nil {
		log.Info("deleting deployment", "deployment", actualDeployment)
		if err := r.Delete(ctx, &actualDeployment); err != nil {
			log.Error(err, "unable to delete Deployment for Deployer", "deployment", actualDeployment)
			return nil, err
		}
		return nil, nil
	}

	// create deployment if it doesn't exist
	if actualDeployment.Name == "" {
		log.Info("creating deployment", "spec", desiredDeployment.Spec)
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
	log.Info("reconciling deployment", "diff", cmp.Diff(actualDeployment.Spec, deployment.Spec))
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
			Labels:       labels,
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-deployer-", deployer.Name),
			Namespace:    deployer.Namespace,
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
	if deployment.Spec.Template.Spec.Containers[0].Image == "" {
		deployment.Spec.Template.Spec.Containers[0].Image = deployer.Status.LatestImage
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

func (r *DeployerReconciler) reconcileIngress(ctx context.Context, log logr.Logger, deployer *corev1alpha1.Deployer, serviceName string) (*networkingv1beta1.Ingress, error) {
	var actualIngress networkingv1beta1.Ingress
	var childIngresses networkingv1beta1.IngressList

	if err := r.List(ctx, &childIngresses, client.InNamespace(deployer.Namespace), client.MatchingField(ingressIndexField, deployer.Name)); err != nil {
		return nil, err
	}

	if len(childIngresses.Items) == 1 {
		actualIngress = childIngresses.Items[0]
	} else if len(childIngresses.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraIngress := range childIngresses.Items {
			log.Info("deleting extra ingress", "ingress", extraIngress)
			if err := r.Delete(ctx, &extraIngress); err != nil {
				return nil, err
			}
		}
	}

	desiredIngress, err := r.constructIngressForDeployer(deployer)
	if err != nil {
		return nil, err
	}

	// delete ingress if no longer needed
	if desiredIngress == nil {
		log.Info("deleting ingress", "ingress", actualIngress)
		if err := r.Delete(ctx, &actualIngress); err != nil {
			log.Error(err, "unable to delete ingress for Deployer", "ingress", actualIngress)
			return nil, err
		}
		return nil, nil
	}

	// create ingress if it doesn't exist
	if actualIngress.Name == "" {
		log.Info("creating service", "spec", desiredIngress.Spec)
		if err := r.Create(ctx, desiredIngress); err != nil {
			log.Error(err, "unable to create Ingress for Deployer", "ingress", desiredIngress)
			return nil, err
		}
		return desiredIngress, nil
	}

	if r.ingressSemanticEquals(desiredIngress, &actualIngress) {
		// ingress is unchanged
		return &actualIngress, nil
	}

	// update ingress with desired changes
	ingress := actualIngress.DeepCopy()
	ingress.ObjectMeta.Labels = desiredIngress.ObjectMeta.Labels
	ingress.Spec = desiredIngress.Spec
	log.Info("reconciling ingress", "diff", cmp.Diff(actualIngress.Spec, ingress.Spec))
	if err := r.Update(ctx, ingress); err != nil {
		log.Error(err, "unable to update Ingress for Deployer", "deployment", ingress)
		return nil, err
	}

	return ingress, nil
}

func (r *DeployerReconciler) ingressSemanticEquals(desiredIngress, ingress *networkingv1beta1.Ingress) bool {
	return equality.Semantic.DeepEqual(desiredIngress.Spec, ingress.Spec) &&
		equality.Semantic.DeepEqual(desiredIngress.ObjectMeta.Labels, ingress.ObjectMeta.Labels)
}

func (r *DeployerReconciler) constructIngressForDeployer(deployer *corev1alpha1.Deployer) (*networkingv1beta1.Ingress, error) {
	// construct ingress if service is present
	if deployer.Status.ServiceName == "" {
		return nil, nil
	}
	labels := r.constructLabelsForDeployer(deployer)

	ingress := &networkingv1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-deployer-", deployer.Name),
			Namespace:    deployer.Namespace,
		},
		Spec: networkingv1beta1.IngressSpec{
			Rules: []networkingv1beta1.IngressRule{{
				Host: fmt.Sprintf("%s.%s.%s", deployer.Status.ServiceName, deployer.Namespace, domain),
				IngressRuleValue: networkingv1beta1.IngressRuleValue{
					HTTP: &networkingv1beta1.HTTPIngressRuleValue{
						Paths: []networkingv1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: networkingv1beta1.IngressBackend{
								ServiceName: deployer.Status.ServiceName,
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	}

	if err := ctrl.SetControllerReference(deployer, ingress, r.Scheme); err != nil {
		return nil, err
	}

	return ingress, nil
}

func (r *DeployerReconciler) reconcileChildService(ctx context.Context, log logr.Logger, deployer *corev1alpha1.Deployer) (*corev1.Service, error) {
	var actualService corev1.Service
	var childServices corev1.ServiceList
	if err := r.List(ctx, &childServices, client.InNamespace(deployer.Namespace), client.MatchingField(serviceIndexField, deployer.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childServices.Items) == 1 {
		actualService = childServices.Items[0]
	} else if len(childServices.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraService := range childServices.Items {
			log.Info("deleting extra service", "service", extraService)
			if err := r.Delete(ctx, &extraService); err != nil {
				return nil, err
			}
		}
	}

	desiredService, err := r.constructServiceForDeployer(deployer)
	if err != nil {
		return nil, err
	}

	// delete service if no longer needed
	if desiredService == nil {
		log.Info("deleting service", "service", actualService)
		if err := r.Delete(ctx, &actualService); err != nil {
			log.Error(err, "unable to delete Service for Deployer", "service", actualService)
			return nil, err
		}
		return nil, nil
	}

	// create service if it doesn't exist
	if actualService.Name == "" {
		log.Info("creating service", "spec", desiredService.Spec)
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
	log.Info("reconciling service", "diff", cmp.Diff(actualService.Spec, service.Spec))
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
			Labels:       labels,
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-deployer-", deployer.Name),
			Namespace:    deployer.Namespace,
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

	if err := controllers.IndexControllersOfType(mgr, deploymentIndexField, &corev1alpha1.Deployer{}, &appsv1.Deployment{}); err != nil {
		return err
	}
	if err := controllers.IndexControllersOfType(mgr, serviceIndexField, &corev1alpha1.Deployer{}, &corev1.Service{}); err != nil {
		return err
	}
	if err := controllers.IndexControllersOfType(mgr, ingressIndexField, &corev1alpha1.Deployer{}, &networkingv1beta1.Ingress{}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.Deployer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1beta1.Ingress{}).
		// watch for build mutations to update dependent deployers
		Watches(&source.Kind{Type: &buildv1alpha1.Application{}}, enqueueTrackedResources(&buildv1alpha1.Application{})).
		Watches(&source.Kind{Type: &buildv1alpha1.Container{}}, enqueueTrackedResources(&buildv1alpha1.Container{})).
		Watches(&source.Kind{Type: &buildv1alpha1.Function{}}, enqueueTrackedResources(&buildv1alpha1.Function{})).
		Complete(r)
}
