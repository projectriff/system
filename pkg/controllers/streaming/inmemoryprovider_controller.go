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
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/source"

	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
	"github.com/projectriff/system/pkg/controllers"
	"github.com/projectriff/system/pkg/refs"
	"github.com/projectriff/system/pkg/tracker"
)

const (
	inMemoryProviderDeploymentIndexField = ".metadata.inMemoryProviderDeploymentController"
	inMemoryProviderServiceIndexField    = ".metadata.inMemoryProviderServiceController"
)

// InMemoryProviderReconciler reconciles a InMemoryProvider object
type InMemoryProviderReconciler struct {
	client.Client
	Recorder  record.EventRecorder
	Log       logr.Logger
	Scheme    *runtime.Scheme
	Tracker   tracker.Tracker
	Namespace string
}

// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=inmemoryproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=inmemoryproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

func (r *InMemoryProviderReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("inmemoryprovider", req.NamespacedName)

	// your logic here
	var inMemoryProvider streamingv1alpha1.InMemoryProvider
	if err := r.Get(ctx, req.NamespacedName, &inMemoryProvider); err != nil {
		log.Error(err, "unable to fetch InMemoryProvider")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, ignoreNotFound(err)
	}

	originalInMemoryProvider := inMemoryProvider.DeepCopy()
	inMemoryProvider.Default()
	inMemoryProvider.Status.InitializeConditions()

	result, err := r.reconcile(ctx, log, &inMemoryProvider)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(inMemoryProvider.Status, originalInMemoryProvider.Status) {
		// update status
		log.Info("updating inmemory provider status", "diff", cmp.Diff(originalInMemoryProvider.Status, inMemoryProvider.Status))
		if updateErr := r.Status().Update(ctx, &inMemoryProvider); updateErr != nil {
			log.Error(updateErr, "unable to update InMemoryProvider status", "inmemoryprovider", inMemoryProvider)
			r.Recorder.Eventf(&inMemoryProvider, corev1.EventTypeWarning, "StatusUpdateFailed",
				"Failed to update status: %v", updateErr)
			return ctrl.Result{Requeue: true}, updateErr
		}
		r.Recorder.Eventf(&inMemoryProvider, corev1.EventTypeNormal, "StatusUpdated",
			"Updated status")
	}

	return result, err
}

func (r *InMemoryProviderReconciler) reconcile(ctx context.Context, log logr.Logger, inMemoryProvider *streamingv1alpha1.InMemoryProvider) (ctrl.Result, error) {

	// Lookup and track configMap to know which images to use
	cm := corev1.ConfigMap{}
	cmKey := types.NamespacedName{Namespace: r.Namespace, Name: nopProviderImages}
	// track config map for new images
	r.Tracker.Track(
		tracker.NewKey(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, cmKey),
		types.NamespacedName{Namespace: inMemoryProvider.GetNamespace(), Name: inMemoryProvider.GetName()},
	)
	if err := r.Get(ctx, cmKey, &cm); err != nil {
		log.Error(err, "unable to lookup images configMap")
		return ctrl.Result{}, err
	}

	// Reconcile deployment for gateway
	gatewayDeployment, err := r.reconcileGatewayDeployment(ctx, log, inMemoryProvider, &cm)
	if err != nil {
		log.Error(err, "unable to reconcile gateway Deployment", "inmemoryprovider", inMemoryProvider)
		return ctrl.Result{}, err
	}
	inMemoryProvider.Status.GatewayDeploymentRef = refs.NewTypedLocalObjectReferenceForObject(gatewayDeployment, r.Scheme)
	inMemoryProvider.Status.PropagateGatewayDeploymentStatus(&gatewayDeployment.Status)

	// Reconcile service for gateway
	gatewayService, err := r.reconcileGatewayService(ctx, log, inMemoryProvider)
	if err != nil {
		log.Error(err, "unable to reconcile gateway Service", "inmemoryprovider", inMemoryProvider)
		return ctrl.Result{}, err
	}
	inMemoryProvider.Status.GatewayServiceRef = refs.NewTypedLocalObjectReferenceForObject(gatewayService, r.Scheme)
	inMemoryProvider.Status.PropagateGatewayServiceStatus(&gatewayService.Status)

	// Reconcile deployment for provisioner
	provisionerDeployment, err := r.reconcileProvisionerDeployment(ctx, log, inMemoryProvider, &cm)
	if err != nil {
		log.Error(err, "unable to reconcile provisioner Deployment", "inmemoryprovider", inMemoryProvider)
		return ctrl.Result{}, err
	}
	inMemoryProvider.Status.ProvisionerDeploymentRef = refs.NewTypedLocalObjectReferenceForObject(provisionerDeployment, r.Scheme)
	inMemoryProvider.Status.PropagateProvisionerDeploymentStatus(&provisionerDeployment.Status)

	// Reconcile service for provisioner
	provisionerService, err := r.reconcileProvisionerService(ctx, log, inMemoryProvider)
	if err != nil {
		log.Error(err, "unable to reconcile provisioner Service", "inmemoryprovider", inMemoryProvider)
		return ctrl.Result{}, err
	}
	inMemoryProvider.Status.ProvisionerServiceRef = refs.NewTypedLocalObjectReferenceForObject(provisionerService, r.Scheme)
	inMemoryProvider.Status.PropagateProvisionerServiceStatus(&provisionerService.Status)

	return ctrl.Result{}, nil

}

func (r *InMemoryProviderReconciler) reconcileGatewayDeployment(ctx context.Context, log logr.Logger, inMemoryProvider *streamingv1alpha1.InMemoryProvider, cm *corev1.ConfigMap) (*appsv1.Deployment, error) {
	var actualDeployment appsv1.Deployment
	var childDeployments appsv1.DeploymentList
	if err := r.List(ctx, &childDeployments,
		client.InNamespace(inMemoryProvider.Namespace),
		client.MatchingLabels(map[string]string{streamingv1alpha1.InMemoryProviderGatewayLabelKey: inMemoryProvider.Name}),
		client.MatchingField(inMemoryProviderDeploymentIndexField, inMemoryProvider.Name)); err != nil {
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
				r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "DeleteFailed",
					"Failed to delete gateway Deployment %q: %v", extraDeployment.Name, err)
				return nil, err
			}
			r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Deleted",
				"Deleted gateway Deployment %q", extraDeployment.Name)
		}
	}

	gatewayImg := cm.Data[gatewayImageKey]
	if gatewayImg == "" {
		return nil, fmt.Errorf("missing gateway image configuration")
	}

	desiredDeployment, err := r.constructGatewayDeploymentForInMemoryProvider(inMemoryProvider, gatewayImg)
	if err != nil {
		return nil, err
	}

	// delete deployment if no longer needed
	if desiredDeployment == nil {
		if err := r.Delete(ctx, &actualDeployment); err != nil {
			log.Error(err, "unable to delete Deployment for InMemoryProvider", "deployment", actualDeployment)
			r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "DeleteFailed",
				"Failed to delete gateway Deployment %q: %v", actualDeployment.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Deleted",
			"Deleted gateway Deployment %q", actualDeployment.Name)
		return nil, nil
	}

	// create deployment if it doesn't exist
	if actualDeployment.Name == "" {
		log.Info("creating gateway deployment", "spec", desiredDeployment.Spec)
		if err := r.Create(ctx, desiredDeployment); err != nil {
			log.Error(err, "unable to create Deployment for InMemoryProvider", "deployment", desiredDeployment)
			r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "CreationFailed",
				"Failed to create gateway Deployment %q: %v", desiredDeployment.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Created",
			"Created gateway Deployment %q", desiredDeployment.Name)
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
		log.Error(err, "unable to update Deployment for InMemoryProvider", "deployment", deployment)
		r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update gateway Deployment %q: %v", deployment.Name, err)
		return nil, err
	}
	r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Updated",
		"Updated gateway Deployment %q", deployment.Name)

	return deployment, nil
}

func (r *InMemoryProviderReconciler) constructGatewayDeploymentForInMemoryProvider(inMemoryProvider *streamingv1alpha1.InMemoryProvider, gatewayImg string) (*appsv1.Deployment, error) {
	labels := r.constructGatewayLabelsForInMemoryProvider(inMemoryProvider)

	env, err := r.gatewayEnvironmentForInMemoryProvider(inMemoryProvider)
	if err != nil {
		return nil, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-inmemory-gateway-", inMemoryProvider.Name),
			Namespace:    inMemoryProvider.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					streamingv1alpha1.InMemoryProviderGatewayLabelKey: inMemoryProvider.Name,
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
	if err := ctrl.SetControllerReference(inMemoryProvider, deployment, r.Scheme); err != nil {
		return nil, err
	}

	return deployment, nil
}

func (r *InMemoryProviderReconciler) gatewayEnvironmentForInMemoryProvider(inMemoryProvider *streamingv1alpha1.InMemoryProvider) ([]corev1.EnvVar, error) {
	return []corev1.EnvVar{
		{Name: "storage_positions_type", Value: "MEMORY"},
		{Name: "storage_records_type", Value: "MEMORY"},
	}, nil
}

func (r *InMemoryProviderReconciler) constructGatewayLabelsForInMemoryProvider(inMemoryProvider *streamingv1alpha1.InMemoryProvider) map[string]string {
	labels := make(map[string]string, len(inMemoryProvider.ObjectMeta.Labels)+1)
	// pass through existing labels
	for k, v := range inMemoryProvider.ObjectMeta.Labels {
		labels[k] = v
	}

	labels[streamingv1alpha1.InMemoryProviderLabelKey] = inMemoryProvider.Name
	labels[streamingv1alpha1.InMemoryProviderGatewayLabelKey] = inMemoryProvider.Name

	return labels
}

func (r *InMemoryProviderReconciler) deploymentSemanticEquals(desiredDeployment, deployment *appsv1.Deployment) bool {
	return equality.Semantic.DeepEqual(desiredDeployment.Spec, deployment.Spec) &&
		equality.Semantic.DeepEqual(desiredDeployment.ObjectMeta.Labels, deployment.ObjectMeta.Labels)
}

func (r *InMemoryProviderReconciler) reconcileGatewayService(ctx context.Context, log logr.Logger, inMemoryProvider *streamingv1alpha1.InMemoryProvider) (*corev1.Service, error) {
	var actualService corev1.Service
	var childServices corev1.ServiceList
	if err := r.List(ctx, &childServices,
		client.InNamespace(inMemoryProvider.Namespace),
		client.MatchingLabels(map[string]string{streamingv1alpha1.InMemoryProviderGatewayLabelKey: inMemoryProvider.Name}),
		client.MatchingField(inMemoryProviderServiceIndexField, inMemoryProvider.Name)); err != nil {
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
				r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "DeleteFailed",
					"Failed to delete gateway Service %q: %v", extraService.Name, err)
				return nil, err
			}
			r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Deleted",
				"Deleted gateway Service %q", extraService.Name)
		}
	}

	desiredService, err := r.constructGatewayServiceForInMemoryProvider(inMemoryProvider)
	if err != nil {
		return nil, err
	}

	// delete service if no longer needed
	if desiredService == nil {
		if err := r.Delete(ctx, &actualService); err != nil {
			log.Error(err, "unable to delete gateway Service for InMemoryProvider", "service", actualService)
			r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "DeleteFailed",
				"Failed to delete gateway Service %q: %v", actualService.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Deleted",
			"Deleted gateway Service %q", actualService.Name)
		return nil, nil
	}

	// create service if it doesn't exist
	if actualService.Name == "" {
		log.Info("creating gateway service", "spec", desiredService.Spec)
		if err := r.Create(ctx, desiredService); err != nil {
			log.Error(err, "unable to create gateway Service for InMemoryProvider", "service", desiredService)
			r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "CreationFailed",
				"Failed to create gateway Service %q: %v", desiredService.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Created",
			"Created gateway Service %q", desiredService.Name)
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
		log.Error(err, "unable to update gateway Service for InMemoryProvider", "service", service)
		r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update gateway Service %q: %v", service.Name, err)
		return nil, err
	}
	r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Updated",
		"Updated gateway Service %q", service.Name)

	return service, nil
}

func (r *InMemoryProviderReconciler) serviceSemanticEquals(desiredService, service *corev1.Service) bool {
	return equality.Semantic.DeepEqual(desiredService.Spec, service.Spec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels)
}

func (r *InMemoryProviderReconciler) constructGatewayServiceForInMemoryProvider(inMemoryProvider *streamingv1alpha1.InMemoryProvider) (*corev1.Service, error) {
	labels := r.constructGatewayLabelsForInMemoryProvider(inMemoryProvider)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-inmemory-gateway-", inMemoryProvider.Name),
			Namespace:    inMemoryProvider.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "gateway", Port: 6565},
			},
			Selector: map[string]string{
				streamingv1alpha1.InMemoryProviderGatewayLabelKey: inMemoryProvider.Name,
			},
		},
	}
	if err := ctrl.SetControllerReference(inMemoryProvider, service, r.Scheme); err != nil {
		return nil, err
	}

	return service, nil
}

func (r *InMemoryProviderReconciler) reconcileProvisionerDeployment(ctx context.Context, log logr.Logger, inMemoryProvider *streamingv1alpha1.InMemoryProvider, cm *corev1.ConfigMap) (*appsv1.Deployment, error) {
	var actualDeployment appsv1.Deployment
	var childDeployments appsv1.DeploymentList
	if err := r.List(ctx, &childDeployments,
		client.InNamespace(inMemoryProvider.Namespace),
		client.MatchingLabels(map[string]string{streamingv1alpha1.InMemoryProviderProvisionerLabelKey: inMemoryProvider.Name}),
		client.MatchingField(inMemoryProviderDeploymentIndexField, inMemoryProvider.Name)); err != nil {
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
				r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "DeleteFailed",
					"Failed to delete provisioner Deployment %q: %v", extraDeployment.Name, err)
				return nil, err
			}
			r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Deleted",
				"Deleted provisioner Deployment %q", extraDeployment.Name)
		}
	}

	provisionerImg := cm.Data[provisionerImageKey]
	if provisionerImg == "" {
		return nil, fmt.Errorf("missing provisioner image configuration")
	}

	desiredDeployment, err := r.constructProvisionerDeploymentForInMemoryProvider(inMemoryProvider, provisionerImg)
	if err != nil {
		return nil, err
	}

	// delete deployment if no longer needed
	if desiredDeployment == nil {
		if err := r.Delete(ctx, &actualDeployment); err != nil {
			log.Error(err, "unable to delete Deployment for InMemoryProvider", "deployment", actualDeployment)
			r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "DeleteFailed",
				"Failed to delete provisioner Deployment %q: %v", actualDeployment.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Deleted",
			"Deleted provisioner Deployment %q", actualDeployment.Name)
		return nil, nil
	}

	// create deployment if it doesn't exist
	if actualDeployment.Name == "" {
		log.Info("creating provisioner deployment", "spec", desiredDeployment.Spec)
		if err := r.Create(ctx, desiredDeployment); err != nil {
			log.Error(err, "unable to create Deployment for InMemoryProvider", "deployment", desiredDeployment)
			r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "CreationFailed",
				"Failed to create provisioner Deployment %q: %v", desiredDeployment.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Created",
			"Created provisioner Deployment %q", desiredDeployment.Name)
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
		log.Error(err, "unable to update Deployment for InMemoryProvider", "deployment", deployment)
		r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update provisioner Deployment %q: %v", deployment.Name, err)
		return nil, err
	}
	r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Updated",
		"Updated provisioner Deployment %q", deployment.Name)

	return deployment, nil
}

func (r *InMemoryProviderReconciler) constructProvisionerDeploymentForInMemoryProvider(inMemoryProvider *streamingv1alpha1.InMemoryProvider, provisionerImg string) (*appsv1.Deployment, error) {
	labels := r.constructProvisionerLabelsForInMemoryProvider(inMemoryProvider)

	env := []corev1.EnvVar{
		{Name: "GATEWAY", Value: fmt.Sprintf("%s.%s:6565", inMemoryProvider.Status.GatewayServiceRef.Name, inMemoryProvider.Namespace)}, // TODO get port number from svc lookup?
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-inmemory-provisioner-", inMemoryProvider.Name),
			Namespace:    inMemoryProvider.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					streamingv1alpha1.InMemoryProviderProvisionerLabelKey: inMemoryProvider.Name,
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
	if err := ctrl.SetControllerReference(inMemoryProvider, deployment, r.Scheme); err != nil {
		return nil, err
	}

	return deployment, nil
}

func (r *InMemoryProviderReconciler) reconcileProvisionerService(ctx context.Context, log logr.Logger, inMemoryProvider *streamingv1alpha1.InMemoryProvider) (*corev1.Service, error) {
	var actualService corev1.Service
	var childServices corev1.ServiceList
	if err := r.List(ctx, &childServices,
		client.InNamespace(inMemoryProvider.Namespace),
		client.MatchingLabels(map[string]string{streamingv1alpha1.InMemoryProviderProvisionerLabelKey: inMemoryProvider.Name}),
		client.MatchingField(inMemoryProviderServiceIndexField, inMemoryProvider.Name)); err != nil {
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
				r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "DeleteFailed",
					"Failed to delete provisioner Service %q: %v", extraService.Name, err)
				return nil, err
			}
			r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Deleted",
				"Deleted provisioner Service %q", extraService.Name)
		}
	}

	desiredService, err := r.constructProvisionerServiceForInMemoryProvider(inMemoryProvider)
	if err != nil {
		return nil, err
	}

	// delete service if no longer needed
	if desiredService == nil {
		if err := r.Delete(ctx, &actualService); err != nil {
			log.Error(err, "unable to delete provisioner Service for InMemoryProvider", "service", actualService)
			r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "DeleteFailed",
				"Failed to delete provisioner Serice %q: %v", actualService.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Deleted",
			"Deleted provisioner Service %q", actualService.Name)
		return nil, nil
	}

	// create service if it doesn't exist
	if actualService.Name == "" {
		log.Info("creating provisioner service", "spec", desiredService.Spec)
		if err := r.Create(ctx, desiredService); err != nil {
			log.Error(err, "unable to create provisioner Service for InMemoryProvider", "service", desiredService)
			r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "CreationFailed",
				"Failed to create provisioner Service %q: %v", desiredService.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Created",
			"Created provisioner Service %q", desiredService.Name)
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
		log.Error(err, "unable to update provisioner Service for InMemoryProvider", "service", service)
		r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update provisioner Service %q: %v", service.Name, err)
		return nil, err
	}
	r.Recorder.Eventf(inMemoryProvider, corev1.EventTypeNormal, "Updated",
		"Updated provisioner Service %q", service.Name)

	return service, nil
}

func (r *InMemoryProviderReconciler) constructProvisionerServiceForInMemoryProvider(inMemoryProvider *streamingv1alpha1.InMemoryProvider) (*corev1.Service, error) {
	labels := r.constructProvisionerLabelsForInMemoryProvider(inMemoryProvider)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: make(map[string]string),
			Name:        fmt.Sprintf("%s-inmemory-provisioner", inMemoryProvider.Name),
			Namespace:   inMemoryProvider.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
			},
			Selector: map[string]string{
				streamingv1alpha1.InMemoryProviderProvisionerLabelKey: inMemoryProvider.Name,
			},
		},
	}
	if err := ctrl.SetControllerReference(inMemoryProvider, service, r.Scheme); err != nil {
		return nil, err
	}

	return service, nil
}

func (r *InMemoryProviderReconciler) constructProvisionerLabelsForInMemoryProvider(inMemoryProvider *streamingv1alpha1.InMemoryProvider) map[string]string {
	labels := make(map[string]string, len(inMemoryProvider.ObjectMeta.Labels)+2)
	// pass through existing labels
	for k, v := range inMemoryProvider.ObjectMeta.Labels {
		labels[k] = v
	}

	labels[streamingv1alpha1.InMemoryProviderLabelKey] = inMemoryProvider.Name
	labels[streamingv1alpha1.InMemoryProviderProvisionerLabelKey] = inMemoryProvider.Name
	labels[streamingv1alpha1.ProvisionerLabelKey] = streamingv1alpha1.InMemoryProvisioner

	return labels
}

func (r *InMemoryProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := controllers.IndexControllersOfType(mgr, inMemoryProviderDeploymentIndexField, &streamingv1alpha1.InMemoryProvider{}, &appsv1.Deployment{}); err != nil {
		return err
	}
	if err := controllers.IndexControllersOfType(mgr, inMemoryProviderServiceIndexField, &streamingv1alpha1.InMemoryProvider{}, &corev1.Service{}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&streamingv1alpha1.InMemoryProvider{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Watches(&source.Kind{Type: &corev1.ConfigMap{}}, controllers.EnqueueTracked(&corev1.ConfigMap{}, r.Tracker, r.Scheme)).
		Complete(r)
}
