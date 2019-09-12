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
	"errors"
	"fmt"
	"github.com/go-logr/logr"
	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
	"github.com/projectriff/system/pkg/tracker"
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
)

// ProviderReconciler reconciles a Provider object
type ProviderReconciler struct {
	client.Client
	Log       logr.Logger
	Scheme    *runtime.Scheme
	Tracker   tracker.Tracker
	Namespace string
}

// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=providers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=providers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch

func (r *ProviderReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("provider", req.NamespacedName)

	// your logic here
	var provider streamingv1alpha1.Provider
	if err := r.Get(ctx, req.NamespacedName, &provider); err != nil {
		log.Error(err, "unable to fetch Provider")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, ignoreNotFound(err)
	}

	originalProvider := provider.DeepCopy()
	provider.SetDefaults(ctx)
	provider.Status.InitializeConditions()

	result, err := r.reconcile(ctx, log, &provider)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(provider.Status, originalProvider.Status) {
		// update status
		if updateErr := r.Status().Update(ctx, &provider); updateErr != nil {
			log.Error(updateErr, "unable to update Provider status", "provider", provider)
			return ctrl.Result{Requeue: true}, updateErr
		}
	}

	return result, err
}

func (r *ProviderReconciler) reconcile(ctx context.Context, log logr.Logger, provider *streamingv1alpha1.Provider) (ctrl.Result, error) {

	// Lookup and track configMap to know which images to use
	cm := corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: providerImages}, &cm); err != nil {
		log.Error(err, "unable to lookup images configMap")
		return ctrl.Result{}, err
	}
	// track config map for new images
	if err := r.Tracker.Track(&cm, types.NamespacedName{Namespace: provider.GetNamespace(), Name: provider.GetName()}); err != nil {
		log.Error(err, "unable to setup tracking of images configMap")
		return ctrl.Result{}, err
	}

	// Reconcile deployment for liiklus
	liiklusDeployment, err := r.reconcileLiiklusDeployment(ctx, log, provider, &cm)
	if err != nil {
		log.Error(err, "unable to reconcile liiklus Deployment", "provider", provider)
		return ctrl.Result{}, err
	}
	provider.Status.LiiklusDeploymentName = liiklusDeployment.Name
	provider.Status.PropagateLiiklusDeploymentStatus(&liiklusDeployment.Status)

	// Reconcile service for liiklus
	liiklusService, err := r.reconcileLiiklusService(ctx, log, provider)
	if err != nil {
		log.Error(err, "unable to reconcile liiklus Service", "provider", provider)
		return ctrl.Result{}, err
	}
	provider.Status.LiiklusServiceName = liiklusService.Name
	provider.Status.PropagateLiiklusServiceStatus(&liiklusService.Status)

	// Reconcile deployment for provisioner
	provisionerDeployment, err := r.reconcileProvisionerDeployment(ctx, log, provider, &cm)
	if err != nil {
		log.Error(err, "unable to reconcile provisioner Deployment", "provider", provider)
		return ctrl.Result{}, err
	}
	provider.Status.ProvisionerDeploymentName = provisionerDeployment.Name
	provider.Status.PropagateProvisionerDeploymentStatus(&provisionerDeployment.Status)

	// Reconcile service for provisioner
	provisionerService, err := r.reconcileProvisionerService(ctx, log, provider)
	if err != nil {
		log.Error(err, "unable to reconcile provisioner Service", "provider", provider)
		return ctrl.Result{}, err
	}
	provider.Status.ProvisionerServiceName = provisionerService.Name
	provider.Status.PropagateProvisionerServiceStatus(&provisionerService.Status)

	return ctrl.Result{}, nil

}

func (r *ProviderReconciler) reconcileLiiklusDeployment(ctx context.Context, log logr.Logger, provider *streamingv1alpha1.Provider, cm *corev1.ConfigMap) (*appsv1.Deployment, error) {
	var actualDeployment appsv1.Deployment
	if provider.Status.LiiklusDeploymentName != "" {
		if err := r.Get(ctx, types.NamespacedName{Namespace: provider.Namespace, Name: provider.Status.LiiklusDeploymentName}, &actualDeployment); err != nil {
			log.Error(err, "unable to fetch liiklus Deployment")
			if !apierrs.IsNotFound(err) {
				return nil, err
			}
			// reset the DeploymentName since it no longer exists and needs to
			// be recreated
			provider.Status.LiiklusDeploymentName = ""
		}
		// check that the deployment is not controlled by another resource
		if !metav1.IsControlledBy(&actualDeployment, provider) {
			provider.Status.MarkLiiklusDeploymentNotOwned()
			return nil, fmt.Errorf("provider %q does not own Deployment %q", provider.Name, actualDeployment.Name)
		}
	}

	liiklusImg := cm.Data[liiklusImageKey]
	if liiklusImg == "" {
		return nil, fmt.Errorf("missing liiklus image configuration")
	}

	desiredDeployment, err := r.constructLiiklusDeploymentForProvider(provider, liiklusImg)
	if err != nil {
		return nil, err
	}

	// delete deployment if no longer needed
	if desiredDeployment == nil {
		if err := r.Delete(ctx, &actualDeployment); err != nil {
			log.Error(err, "unable to delete Deployment for Provider", "deployment", actualDeployment)
			return nil, err
		}
		return nil, nil
	}

	// create deployment if it doesn't exist
	if provider.Status.LiiklusDeploymentName == "" {
		if err := r.Create(ctx, desiredDeployment); err != nil {
			log.Error(err, "unable to create Deployment for Provider", "deployment", desiredDeployment)
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
		log.Error(err, "unable to update Deployment for Provider", "deployment", deployment)
		return nil, err
	}

	return deployment, nil
}

func (r *ProviderReconciler) constructLiiklusDeploymentForProvider(provider *streamingv1alpha1.Provider, liiklusImg string) (*appsv1.Deployment, error) {
	labels := r.constructLiiklusLabelsForProvider(provider)

	env, err := r.liiklusEnvironmentForProvider(provider)
	if err != nil {
		return nil, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: make(map[string]string),
			Name:        fmt.Sprintf("%s-liiklus", provider.Name),
			Namespace:   provider.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					streamingv1alpha1.ProviderLiiklusLabelKey: provider.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "liiklus",
							Image:           liiklusImg,
							ImagePullPolicy: corev1.PullAlways,
							Env:             env,
						},
					},
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(provider, deployment, r.Scheme); err != nil {
		return nil, err
	}

	return deployment, nil
}

func (r *ProviderReconciler) liiklusEnvironmentForProvider(provider *streamingv1alpha1.Provider) ([]corev1.EnvVar, error) {
	switch provider.Spec.BrokerType {
	case streamingv1alpha1.KafkaBroker:
		if address, ok := provider.Spec.Config["bootstrapServers"]; !ok {
			return nil, errors.New("kafka config missing bootstrapServers config")
		} else {
			return []corev1.EnvVar{
				{Name: "kafka_bootstrapServers", Value: address},
				{Name: "storage_positions_type", Value: "MEMORY"},
				{Name: "storage_records_type", Value: "KAFKA"},
			}, nil
		}
	}
	return nil, fmt.Errorf("unsupported broker type %q", provider.Spec.BrokerType)
}

func (r *ProviderReconciler) constructLiiklusLabelsForProvider(provider *streamingv1alpha1.Provider) map[string]string {
	labels := make(map[string]string, len(provider.ObjectMeta.Labels)+1)
	// pass through existing labels
	for k, v := range provider.ObjectMeta.Labels {
		labels[k] = v
	}

	labels[streamingv1alpha1.ProviderLabelKey] = provider.Name
	labels[streamingv1alpha1.ProviderLiiklusLabelKey] = provider.Name

	return labels
}

func (r *ProviderReconciler) deploymentSemanticEquals(desiredDeployment, deployment *appsv1.Deployment) bool {
	return equality.Semantic.DeepEqual(desiredDeployment.Spec, deployment.Spec) &&
		equality.Semantic.DeepEqual(desiredDeployment.ObjectMeta.Labels, deployment.ObjectMeta.Labels)
}

func (r *ProviderReconciler) reconcileLiiklusService(ctx context.Context, log logr.Logger, provider *streamingv1alpha1.Provider) (*corev1.Service, error) {
	var actualService corev1.Service
	if provider.Status.LiiklusServiceName != "" {
		if err := r.Get(ctx, types.NamespacedName{Namespace: provider.Namespace, Name: provider.Status.LiiklusServiceName}, &actualService); err != nil {
			log.Error(err, "unable to fetch liiklus Service")
			if !apierrs.IsNotFound(err) {
				return nil, err
			}
			// reset the ServiceName since it no longer exists and needs to
			// be recreated
			provider.Status.LiiklusServiceName = ""
		}
		// check that the service is not controlled by another resource
		if !metav1.IsControlledBy(&actualService, provider) {
			provider.Status.MarkLiiklusServiceNotOwned()
			return nil, fmt.Errorf("Provider %q does not own Service %q", provider.Name, actualService.Name)
		}
	}

	desiredService, err := r.constructLiiklusServiceForProvider(provider)
	if err != nil {
		return nil, err
	}

	// delete service if no longer needed
	if desiredService == nil {
		if err := r.Delete(ctx, &actualService); err != nil {
			log.Error(err, "unable to delete liiklus Service for Provider", "service", actualService)
			return nil, err
		}
		return nil, nil
	}

	// create service if it doesn't exist
	if provider.Status.LiiklusServiceName == "" {
		if err := r.Create(ctx, desiredService); err != nil {
			log.Error(err, "unable to create liiklus Service for Provider", "service", desiredService)
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
		log.Error(err, "unable to update liiklus Service for Provider", "service", service)
		return nil, err
	}

	return service, nil
}

func (r *ProviderReconciler) serviceSemanticEquals(desiredService, service *corev1.Service) bool {
	return equality.Semantic.DeepEqual(desiredService.Spec, service.Spec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels)
}

func (r *ProviderReconciler) constructLiiklusServiceForProvider(provider *streamingv1alpha1.Provider) (*corev1.Service, error) {
	labels := r.constructLiiklusLabelsForProvider(provider)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: make(map[string]string),
			Name:        fmt.Sprintf("%s-liiklus", provider.Name),
			Namespace:   provider.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "liiklus", Port: 6565},
			},
			Selector: map[string]string{
				streamingv1alpha1.ProviderLiiklusLabelKey: provider.Name,
			},
		},
	}
	if err := ctrl.SetControllerReference(provider, service, r.Scheme); err != nil {
		return nil, err
	}

	return service, nil
}

func (r *ProviderReconciler) reconcileProvisionerDeployment(ctx context.Context, log logr.Logger, provider *streamingv1alpha1.Provider, cm *corev1.ConfigMap) (*appsv1.Deployment, error) {
	var actualDeployment appsv1.Deployment
	if provider.Status.ProvisionerDeploymentName != "" {
		if err := r.Get(ctx, types.NamespacedName{Namespace: provider.Namespace, Name: provider.Status.ProvisionerDeploymentName}, &actualDeployment); err != nil {
			log.Error(err, "unable to fetch provisioner Deployment")
			if !apierrs.IsNotFound(err) {
				return nil, err
			}
			// reset the DeploymentName since it no longer exists and needs to
			// be recreated
			provider.Status.ProvisionerDeploymentName = ""
		}
		// check that the deployment is not controlled by another resource
		if !metav1.IsControlledBy(&actualDeployment, provider) {
			provider.Status.MarkProvisionerDeploymentNotOwned()
			return nil, fmt.Errorf("provider %q does not own Deployment %q", provider.Name, actualDeployment.Name)
		}
	}

	provisionerImg := cm.Data[provisionerImageKey]
	if provisionerImg == "" {
		return nil, fmt.Errorf("missing provisioner image configuration")
	}

	desiredDeployment, err := r.constructProvisionerDeploymentForProvider(provider, provisionerImg)
	if err != nil {
		return nil, err
	}

	// delete deployment if no longer needed
	if desiredDeployment == nil {
		if err := r.Delete(ctx, &actualDeployment); err != nil {
			log.Error(err, "unable to delete Deployment for Provider", "deployment", actualDeployment)
			return nil, err
		}
		return nil, nil
	}

	// create deployment if it doesn't exist
	if provider.Status.ProvisionerDeploymentName == "" {
		if err := r.Create(ctx, desiredDeployment); err != nil {
			log.Error(err, "unable to create Deployment for Provider", "deployment", desiredDeployment)
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
		log.Error(err, "unable to update Deployment for Provider", "deployment", deployment)
		return nil, err
	}

	return deployment, nil
}

func (r *ProviderReconciler) constructProvisionerDeploymentForProvider(provider *streamingv1alpha1.Provider, provisionerImg string) (*appsv1.Deployment, error) {
	labels := r.constructProvisionerLabelsForProvider(provider)

	switch provider.Spec.BrokerType {
	case streamingv1alpha1.KafkaBroker:
		env := []corev1.EnvVar{
			{Name: "GATEWAY", Value: provider.Status.LiiklusServiceName + ":6565"}, // TODO get port numnber from svc lookup
			{Name: "BROKER", Value: provider.Spec.Config["bootstrapServers"]},
		}
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      labels,
				Annotations: make(map[string]string),
				Name:        fmt.Sprintf("%s-provisioner", provider.Name),
				Namespace:   provider.Namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						streamingv1alpha1.ProviderProvisionerLabelKey: provider.Name,
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
		if err := ctrl.SetControllerReference(provider, deployment, r.Scheme); err != nil {
			return nil, err
		}
		return deployment, nil
	}
	return nil, fmt.Errorf("unsupported broker type %q", provider.Spec.BrokerType)

}

func (r *ProviderReconciler) reconcileProvisionerService(ctx context.Context, log logr.Logger, provider *streamingv1alpha1.Provider) (*corev1.Service, error) {
	var actualService corev1.Service
	if provider.Status.ProvisionerServiceName != "" {
		if err := r.Get(ctx, types.NamespacedName{Namespace: provider.Namespace, Name: provider.Status.ProvisionerServiceName}, &actualService); err != nil {
			log.Error(err, "unable to fetch provisioner Service")
			if !apierrs.IsNotFound(err) {
				return nil, err
			}
			// reset the ServiceName since it no longer exists and needs to
			// be recreated
			provider.Status.ProvisionerServiceName = ""
		}
		// check that the service is not controlled by another resource
		if !metav1.IsControlledBy(&actualService, provider) {
			provider.Status.MarkProvisionerServiceNotOwned()
			return nil, fmt.Errorf("Provider %q does not own Service %q", provider.Name, actualService.Name)
		}
	}

	desiredService, err := r.constructProvisionerServiceForProvider(provider)
	if err != nil {
		return nil, err
	}

	// delete service if no longer needed
	if desiredService == nil {
		if err := r.Delete(ctx, &actualService); err != nil {
			log.Error(err, "unable to delete provisioner Service for Provider", "service", actualService)
			return nil, err
		}
		return nil, nil
	}

	// create service if it doesn't exist
	if provider.Status.ProvisionerServiceName == "" {
		if err := r.Create(ctx, desiredService); err != nil {
			log.Error(err, "unable to create provisioner Service for Provider", "service", desiredService)
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
		log.Error(err, "unable to update provisioner Service for Provider", "service", service)
		return nil, err
	}

	return service, nil
}

func (r *ProviderReconciler) constructProvisionerServiceForProvider(provider *streamingv1alpha1.Provider) (*corev1.Service, error) {
	labels := r.constructProvisionerLabelsForProvider(provider)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: make(map[string]string),
			Name:        fmt.Sprintf("%s-provisioner", provider.Name),
			Namespace:   provider.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
			},
			Selector: map[string]string{
				streamingv1alpha1.ProviderProvisionerLabelKey: provider.Name,
			},
		},
	}
	if err := ctrl.SetControllerReference(provider, service, r.Scheme); err != nil {
		return nil, err
	}

	return service, nil
}

func (r *ProviderReconciler) constructProvisionerLabelsForProvider(provider *streamingv1alpha1.Provider) map[string]string {
	labels := make(map[string]string, len(provider.ObjectMeta.Labels)+2)
	// pass through existing labels
	for k, v := range provider.ObjectMeta.Labels {
		labels[k] = v
	}

	labels[streamingv1alpha1.ProviderLabelKey] = provider.Name
	labels[streamingv1alpha1.ProviderProvisionerLabelKey] = provider.Name

	return labels
}

func (r *ProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	enqueueTrackedResources := &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
			requests := []reconcile.Request{}
			if a.Meta.GetNamespace() == r.Namespace && a.Meta.GetName() == providerImages {
				for _, item := range r.Tracker.Lookup(a.Object.(metav1.ObjectMetaAccessor)) {
					requests = append(requests, reconcile.Request{NamespacedName: item})
				}
			}
			return requests
		}),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&streamingv1alpha1.Provider{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Watches(&source.Kind{Type: &corev1.ConfigMap{}}, enqueueTrackedResources).
		Complete(r)
}
