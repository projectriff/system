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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
	"github.com/projectriff/system/pkg/controllers"
	"github.com/projectriff/system/pkg/tracker"
)

const (
	pulsarProviderDeploymentIndexField = ".metadata.pulsarProviderDeploymentController"
	pulsarProviderServiceIndexField    = ".metadata.pulsarProviderServiceController"
)

// PulsarProviderReconciler reconciles a PulsarProvider object
type PulsarProviderReconciler struct {
	client.Client
	Log       logr.Logger
	Scheme    *runtime.Scheme
	Tracker   tracker.Tracker
	Namespace string
}

// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=pulsarproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=pulsarproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch

func (r *PulsarProviderReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("pulsarprovider", req.NamespacedName)

	var pulsarProvider streamingv1alpha1.PulsarProvider
	if err := r.Get(ctx, req.NamespacedName, &pulsarProvider); err != nil {
		log.Error(err, "unable to fetch PulsarProvider")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, ignoreNotFound(err)
	}

	originalPulsarProvider := pulsarProvider.DeepCopy()
	pulsarProvider.Default()
	pulsarProvider.Status.InitializeConditions()

	result, err := r.reconcile(ctx, log, &pulsarProvider)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(pulsarProvider.Status, originalPulsarProvider.Status) {
		// update status
		log.Info("updating pulsar provider status", "diff", cmp.Diff(originalPulsarProvider.Status, pulsarProvider.Status))
		if updateErr := r.Status().Update(ctx, &pulsarProvider); updateErr != nil {
			log.Error(updateErr, "unable to update PulsarProvider status")
			return ctrl.Result{Requeue: true}, updateErr
		}
	}

	return result, err
}

func (r *PulsarProviderReconciler) reconcile(ctx context.Context, log logr.Logger, pulsarProvider *streamingv1alpha1.PulsarProvider) (ctrl.Result, error) {

	// Lookup and track configMap to know which images to use
	cm := corev1.ConfigMap{}
	cmKey := types.NamespacedName{Namespace: r.Namespace, Name: pulsarProviderImages}
	// track config map for new images
	r.Tracker.Track(
		tracker.NewKey(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, cmKey),
		types.NamespacedName{Namespace: pulsarProvider.GetNamespace(), Name: pulsarProvider.GetName()},
	)
	if err := r.Get(ctx, cmKey, &cm); err != nil {
		log.Error(err, "unable to lookup images configMap")
		return ctrl.Result{}, err
	}

	// Reconcile deployment for gateway
	gatewayDeployment, err := r.reconcileGatewayDeployment(ctx, log, pulsarProvider, &cm)
	if err != nil {
		log.Error(err, "unable to reconcile gateway Deployment")
		return ctrl.Result{}, err
	}
	pulsarProvider.Status.GatewayDeploymentName = gatewayDeployment.Name
	pulsarProvider.Status.PropagateGatewayDeploymentStatus(&gatewayDeployment.Status)

	// Reconcile service for gateway
	gatewayService, err := r.reconcileGatewayService(ctx, log, pulsarProvider)
	if err != nil {
		log.Error(err, "unable to reconcile gateway Service")
		return ctrl.Result{}, err
	}
	pulsarProvider.Status.GatewayServiceName = gatewayService.Name
	pulsarProvider.Status.PropagateGatewayServiceStatus(&gatewayService.Status)

	// Reconcile deployment for provisioner
	provisionerDeployment, err := r.reconcileProvisionerDeployment(ctx, log, pulsarProvider, &cm)
	if err != nil {
		log.Error(err, "unable to reconcile provisioner Deployment")
		return ctrl.Result{}, err
	}
	pulsarProvider.Status.ProvisionerDeploymentName = provisionerDeployment.Name
	pulsarProvider.Status.PropagateProvisionerDeploymentStatus(&provisionerDeployment.Status)

	// Reconcile service for provisioner
	provisionerService, err := r.reconcileProvisionerService(ctx, log, pulsarProvider)
	if err != nil {
		log.Error(err, "unable to reconcile provisioner Service")
		return ctrl.Result{}, err
	}
	pulsarProvider.Status.ProvisionerServiceName = provisionerService.Name
	pulsarProvider.Status.PropagateProvisionerServiceStatus(&provisionerService.Status)

	return ctrl.Result{}, nil

}

func (r *PulsarProviderReconciler) reconcileGatewayDeployment(ctx context.Context, log logr.Logger, pulsarProvider *streamingv1alpha1.PulsarProvider, cm *corev1.ConfigMap) (*appsv1.Deployment, error) {
	var actualDeployment appsv1.Deployment
	var childDeployments appsv1.DeploymentList
	if err := r.List(ctx, &childDeployments,
		client.InNamespace(pulsarProvider.Namespace),
		client.MatchingLabels(map[string]string{streamingv1alpha1.PulsarProviderGatewayLabelKey: pulsarProvider.Name}),
		client.MatchingField(pulsarProviderDeploymentIndexField, pulsarProvider.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childDeployments.Items) == 1 {
		actualDeployment = childDeployments.Items[0]
	} else if len(childDeployments.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraDeployment := range childDeployments.Items {
			log.Info("deleting extra gateway deployment", "deployment", extraDeployment)
			if err := r.Delete(ctx, &extraDeployment); err != nil {
				return nil, err
			}
		}
	}

	gatewayImg := cm.Data[gatewayImageKey]
	if gatewayImg == "" {
		return nil, fmt.Errorf("missing gateway image configuration")
	}

	desiredDeployment, err := r.constructGatewayDeploymentForPulsarProvider(pulsarProvider, gatewayImg)
	if err != nil {
		return nil, err
	}

	// delete deployment if no longer needed
	if desiredDeployment == nil {
		if err := r.Delete(ctx, &actualDeployment); err != nil {
			log.Error(err, "unable to delete Deployment for PulsarProvider", "deployment", actualDeployment)
			return nil, err
		}
		return nil, nil
	}

	// create deployment if it doesn't exist
	if actualDeployment.Name == "" {
		log.Info("creating gateway deployment", "spec", desiredDeployment.Spec)
		if err := r.Create(ctx, desiredDeployment); err != nil {
			log.Error(err, "unable to create Deployment for PulsarProvider", "deployment", desiredDeployment)
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
	log.Info("reconciling gateway deployment", "diff", cmp.Diff(actualDeployment.Spec, deployment.Spec))
	if err := r.Update(ctx, deployment); err != nil {
		log.Error(err, "unable to update Deployment for PulsarProvider", "deployment", deployment)
		return nil, err
	}

	return deployment, nil
}

func (r *PulsarProviderReconciler) constructGatewayDeploymentForPulsarProvider(pulsarProvider *streamingv1alpha1.PulsarProvider, gatewayImg string) (*appsv1.Deployment, error) {
	labels := r.constructGatewayLabelsForPulsarProvider(pulsarProvider)

	env, err := r.gatewayEnvironmentForPulsarProvider(pulsarProvider)
	if err != nil {
		return nil, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-pulsar-gateway-", pulsarProvider.Name),
			Namespace:    pulsarProvider.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					streamingv1alpha1.PulsarProviderGatewayLabelKey: pulsarProvider.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "gateway",
							Image:           gatewayImg,
							ImagePullPolicy: corev1.PullAlways,
							Env:             env,
						},
					},
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(pulsarProvider, deployment, r.Scheme); err != nil {
		return nil, err
	}

	return deployment, nil
}

func (r *PulsarProviderReconciler) gatewayEnvironmentForPulsarProvider(pulsarProvider *streamingv1alpha1.PulsarProvider) ([]corev1.EnvVar, error) {
	return []corev1.EnvVar{
		{Name: "storage_records_type", Value: "PULSAR"},
		{Name: "pulsar_serviceUrl", Value: pulsarProvider.Spec.ServiceURL},
		{Name: "storage_positions_type", Value: "MEMORY"},
	}, nil
}

func (r *PulsarProviderReconciler) constructGatewayLabelsForPulsarProvider(pulsarProvider *streamingv1alpha1.PulsarProvider) map[string]string {
	labels := make(map[string]string, len(pulsarProvider.ObjectMeta.Labels)+1)
	// pass through existing labels
	for k, v := range pulsarProvider.ObjectMeta.Labels {
		labels[k] = v
	}

	labels[streamingv1alpha1.PulsarProviderLabelKey] = pulsarProvider.Name
	labels[streamingv1alpha1.PulsarProviderGatewayLabelKey] = pulsarProvider.Name

	return labels
}

func (r *PulsarProviderReconciler) deploymentSemanticEquals(desiredDeployment, deployment *appsv1.Deployment) bool {
	return equality.Semantic.DeepEqual(desiredDeployment.Spec, deployment.Spec) &&
		equality.Semantic.DeepEqual(desiredDeployment.ObjectMeta.Labels, deployment.ObjectMeta.Labels)
}

func (r *PulsarProviderReconciler) reconcileGatewayService(ctx context.Context, log logr.Logger, pulsarProvider *streamingv1alpha1.PulsarProvider) (*corev1.Service, error) {
	var actualService corev1.Service
	var childServices corev1.ServiceList
	if err := r.List(ctx, &childServices,
		client.InNamespace(pulsarProvider.Namespace),
		client.MatchingLabels(map[string]string{streamingv1alpha1.PulsarProviderGatewayLabelKey: pulsarProvider.Name}),
		client.MatchingField(pulsarProviderServiceIndexField, pulsarProvider.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childServices.Items) == 1 {
		actualService = childServices.Items[0]
	} else if len(childServices.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraService := range childServices.Items {
			log.Info("deleting extra gateway service", "service", extraService)
			if err := r.Delete(ctx, &extraService); err != nil {
				return nil, err
			}
		}
	}

	desiredService, err := r.constructGatewayServiceForPulsarProvider(pulsarProvider)
	if err != nil {
		return nil, err
	}

	// delete service if no longer needed
	if desiredService == nil {
		if err := r.Delete(ctx, &actualService); err != nil {
			log.Error(err, "unable to delete gateway Service for PulsarProvider", "service", actualService)
			return nil, err
		}
		return nil, nil
	}

	// create service if it doesn't exist
	if actualService.Name == "" {
		log.Info("creating gateway service", "spec", desiredService.Spec)
		if err := r.Create(ctx, desiredService); err != nil {
			log.Error(err, "unable to create gateway Service for PulsarProvider", "service", desiredService)
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
	log.Info("reconciling gateway service", "diff", cmp.Diff(actualService.Spec, service.Spec))
	if err := r.Update(ctx, service); err != nil {
		log.Error(err, "unable to update gateway Service for PulsarProvider", "service", service)
		return nil, err
	}

	return service, nil
}

func (r *PulsarProviderReconciler) serviceSemanticEquals(desiredService, service *corev1.Service) bool {
	return equality.Semantic.DeepEqual(desiredService.Spec, service.Spec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels)
}

func (r *PulsarProviderReconciler) constructGatewayServiceForPulsarProvider(pulsarProvider *streamingv1alpha1.PulsarProvider) (*corev1.Service, error) {
	labels := r.constructGatewayLabelsForPulsarProvider(pulsarProvider)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-pulsar-gateway-", pulsarProvider.Name),
			Namespace:    pulsarProvider.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "gateway", Port: 6565},
			},
			Selector: map[string]string{
				streamingv1alpha1.PulsarProviderGatewayLabelKey: pulsarProvider.Name,
			},
		},
	}
	if err := ctrl.SetControllerReference(pulsarProvider, service, r.Scheme); err != nil {
		return nil, err
	}

	return service, nil
}

func (r *PulsarProviderReconciler) reconcileProvisionerDeployment(ctx context.Context, log logr.Logger, pulsarProvider *streamingv1alpha1.PulsarProvider, cm *corev1.ConfigMap) (*appsv1.Deployment, error) {
	var actualDeployment appsv1.Deployment
	var childDeployments appsv1.DeploymentList
	if err := r.List(ctx, &childDeployments,
		client.InNamespace(pulsarProvider.Namespace),
		client.MatchingLabels(map[string]string{streamingv1alpha1.PulsarProviderProvisionerLabelKey: pulsarProvider.Name}),
		client.MatchingField(pulsarProviderDeploymentIndexField, pulsarProvider.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childDeployments.Items) == 1 {
		actualDeployment = childDeployments.Items[0]
	} else if len(childDeployments.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraDeployment := range childDeployments.Items {
			log.Info("deleting extra provisioner deployment", "deployment", extraDeployment)
			if err := r.Delete(ctx, &extraDeployment); err != nil {
				return nil, err
			}
		}
	}

	provisionerImg := cm.Data[provisionerImageKey]
	if provisionerImg == "" {
		return nil, fmt.Errorf("missing provisioner image configuration")
	}

	desiredDeployment, err := r.constructProvisionerDeploymentForPulsarProvider(pulsarProvider, provisionerImg)
	if err != nil {
		return nil, err
	}

	// delete deployment if no longer needed
	if desiredDeployment == nil {
		if err := r.Delete(ctx, &actualDeployment); err != nil {
			log.Error(err, "unable to delete Deployment for PulsarProvider", "deployment", actualDeployment)
			return nil, err
		}
		return nil, nil
	}

	// create deployment if it doesn't exist
	if actualDeployment.Name == "" {
		log.Info("creating provisioner deployment", "spec", desiredDeployment.Spec)
		if err := r.Create(ctx, desiredDeployment); err != nil {
			log.Error(err, "unable to create Deployment for PulsarProvider", "deployment", desiredDeployment)
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
	log.Info("reconciling provisioner deployment", "diff", cmp.Diff(actualDeployment.Spec, deployment.Spec))
	if err := r.Update(ctx, deployment); err != nil {
		log.Error(err, "unable to update Deployment for PulsarProvider", "deployment", deployment)
		return nil, err
	}

	return deployment, nil
}

func (r *PulsarProviderReconciler) constructProvisionerDeploymentForPulsarProvider(pulsarProvider *streamingv1alpha1.PulsarProvider, provisionerImg string) (*appsv1.Deployment, error) {
	labels := r.constructProvisionerLabelsForPulsarProvider(pulsarProvider)

	env := []corev1.EnvVar{
		{Name: "GATEWAY", Value: fmt.Sprintf("%s.%s:6565", pulsarProvider.Status.GatewayServiceName, pulsarProvider.Namespace)}, // TODO get port number from svc lookup?
		{Name: "BROKER", Value: pulsarProvider.Spec.ServiceURL},
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-pulsar-provisioner-", pulsarProvider.Name),
			Namespace:    pulsarProvider.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					streamingv1alpha1.PulsarProviderProvisionerLabelKey: pulsarProvider.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "main",
							Image:           provisionerImg,
							ImagePullPolicy: corev1.PullAlways,
							Env:             env,
						},
					},
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(pulsarProvider, deployment, r.Scheme); err != nil {
		return nil, err
	}

	return deployment, nil
}

func (r *PulsarProviderReconciler) reconcileProvisionerService(ctx context.Context, log logr.Logger, pulsarProvider *streamingv1alpha1.PulsarProvider) (*corev1.Service, error) {
	var actualService corev1.Service
	var childServices corev1.ServiceList
	if err := r.List(ctx, &childServices,
		client.InNamespace(pulsarProvider.Namespace),
		client.MatchingLabels(map[string]string{streamingv1alpha1.PulsarProviderProvisionerLabelKey: pulsarProvider.Name}),
		client.MatchingField(pulsarProviderServiceIndexField, pulsarProvider.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childServices.Items) == 1 {
		actualService = childServices.Items[0]
	} else if len(childServices.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraService := range childServices.Items {
			log.Info("deleting extra provisioner service", "service", extraService)
			if err := r.Delete(ctx, &extraService); err != nil {
				return nil, err
			}
		}
	}

	desiredService, err := r.constructProvisionerServiceForPulsarProvider(pulsarProvider)
	if err != nil {
		return nil, err
	}

	// delete service if no longer needed
	if desiredService == nil {
		if err := r.Delete(ctx, &actualService); err != nil {
			log.Error(err, "unable to delete provisioner Service for PulsarProvider", "service", actualService)
			return nil, err
		}
		return nil, nil
	}

	// create service if it doesn't exist
	if actualService.Name == "" {
		log.Info("creating provisioner service", "spec", desiredService.Spec)
		if err := r.Create(ctx, desiredService); err != nil {
			log.Error(err, "unable to create provisioner Service for PulsarProvider", "service", desiredService)
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
	log.Info("reconciling provisioner service", "diff", cmp.Diff(actualService.Spec, service.Spec))
	if err := r.Update(ctx, service); err != nil {
		log.Error(err, "unable to update provisioner Service for PulsarProvider", "service", service)
		return nil, err
	}

	return service, nil
}

func (r *PulsarProviderReconciler) constructProvisionerServiceForPulsarProvider(pulsarProvider *streamingv1alpha1.PulsarProvider) (*corev1.Service, error) {
	labels := r.constructProvisionerLabelsForPulsarProvider(pulsarProvider)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: make(map[string]string),
			Name:        fmt.Sprintf("%s-pulsar-provisioner", pulsarProvider.Name),
			Namespace:   pulsarProvider.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
			},
			Selector: map[string]string{
				streamingv1alpha1.PulsarProviderProvisionerLabelKey: pulsarProvider.Name,
			},
		},
	}
	if err := ctrl.SetControllerReference(pulsarProvider, service, r.Scheme); err != nil {
		return nil, err
	}

	return service, nil
}

func (r *PulsarProviderReconciler) constructProvisionerLabelsForPulsarProvider(pulsarProvider *streamingv1alpha1.PulsarProvider) map[string]string {
	labels := make(map[string]string, len(pulsarProvider.ObjectMeta.Labels)+3)
	// pass through existing labels
	for k, v := range pulsarProvider.ObjectMeta.Labels {
		labels[k] = v
	}

	labels[streamingv1alpha1.PulsarProviderLabelKey] = pulsarProvider.Name
	labels[streamingv1alpha1.PulsarProviderProvisionerLabelKey] = pulsarProvider.Name
	labels[streamingv1alpha1.ProvisionerLabelKey] = streamingv1alpha1.PulsarProvisioner

	return labels
}

func (r *PulsarProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	enqueueTrackedResources := &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
			requests := []reconcile.Request{}
			if a.Meta.GetNamespace() == r.Namespace && a.Meta.GetName() == pulsarProviderImages {
				key := tracker.NewKey(
					schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
					types.NamespacedName{Namespace: a.Meta.GetNamespace(), Name: a.Meta.GetName()},
				)
				for _, item := range r.Tracker.Lookup(key) {
					requests = append(requests, reconcile.Request{NamespacedName: item})
				}
			}
			return requests
		}),
	}

	if err := controllers.IndexControllersOfType(mgr, pulsarProviderDeploymentIndexField, &streamingv1alpha1.PulsarProvider{}, &appsv1.Deployment{}); err != nil {
		return err
	}
	if err := controllers.IndexControllersOfType(mgr, pulsarProviderServiceIndexField, &streamingv1alpha1.PulsarProvider{}, &corev1.Service{}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&streamingv1alpha1.PulsarProvider{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Watches(&source.Kind{Type: &corev1.ConfigMap{}}, enqueueTrackedResources).
		Complete(r)
}
