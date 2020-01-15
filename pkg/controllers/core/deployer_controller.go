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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/projectriff/system/pkg/apis"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	corev1alpha1 "github.com/projectriff/system/pkg/apis/core/v1alpha1"
	"github.com/projectriff/system/pkg/controllers"
	"github.com/projectriff/system/pkg/refs"
	"github.com/projectriff/system/pkg/tracker"
)

const (
	deploymentIndexField = ".metadata.deploymentController"
	serviceIndexField    = ".metadata.serviceController"
	ingressIndexField    = ".metadata.ingressController"
)

// DeployerReconciler reconciles a Deployer object
type DeployerReconciler struct {
	controllers.Config

	deploymentReconciler controllers.SubReconciler
	serviceReconciler    controllers.SubReconciler
	ingressReconciler    controllers.SubReconciler
}

// +kubebuilder:rbac:groups=core.projectriff.io,resources=deployers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.projectriff.io,resources=deployers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=build.projectriff.io,resources=applications;containers;functions,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

func (r *DeployerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("deployer", req.NamespacedName)

	// TODO move to an init or constructor
	if r.deploymentReconciler == nil {
		r.deploymentReconciler = DeployerChildDeploymentReconciler(r)
	}
	if r.serviceReconciler == nil {
		r.serviceReconciler = DeployerChildServiceReconciler(r)
	}
	if r.ingressReconciler == nil {
		r.ingressReconciler = NewDeployerChildIngressReconciler(r)
	}

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
	if !result.Requeue && !equality.Semantic.DeepEqual(deployer.Status, originalDeployer.Status) && deployer.GetDeletionTimestamp() == nil {
		// update status
		log.Info("updating deployer status", "diff", cmp.Diff(originalDeployer.Status, deployer.Status))
		if updateErr := r.Status().Update(ctx, &deployer); updateErr != nil {
			log.Error(updateErr, "unable to update Deployer status", "deployer", deployer)
			r.Recorder.Eventf(&deployer, corev1.EventTypeWarning, "StatusUpdateFailed",
				"Failed to update status: %v", updateErr)
			return ctrl.Result{Requeue: true}, updateErr
		}
		r.Recorder.Eventf(&deployer, corev1.EventTypeNormal, "StatusUpdated",
			"Updated status")
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
	if _, err := r.deploymentReconciler.Reconcile(ctx, deployer); err != nil {
		return ctrl.Result{}, err
	}

	// reconcile service
	if _, err := r.serviceReconciler.Reconcile(ctx, deployer); err != nil {
		return ctrl.Result{}, err
	}

	// reconcile ingress
	if _, err := r.ingressReconciler.Reconcile(ctx, deployer); err != nil {
		return ctrl.Result{}, err
	}

	deployer.Status.ObservedGeneration = deployer.Generation
	return ctrl.Result{}, nil
}

func (r *DeployerReconciler) reconcileBuildImage(ctx context.Context, log logr.Logger, deployer *corev1alpha1.Deployer) error {
	build := deployer.Spec.Build
	if build == nil {
		deployer.Status.LatestImage = deployer.Spec.Template.Spec.Containers[0].Image
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
		Watches(&source.Kind{Type: &corev1.ConfigMap{}}, controllers.EnqueueTracked(&corev1.ConfigMap{}, r.Tracker, r.Scheme)).
		Watches(&source.Kind{Type: &buildv1alpha1.Application{}}, controllers.EnqueueTracked(&buildv1alpha1.Application{}, r.Tracker, r.Scheme)).
		Watches(&source.Kind{Type: &buildv1alpha1.Container{}}, controllers.EnqueueTracked(&buildv1alpha1.Container{}, r.Tracker, r.Scheme)).
		Watches(&source.Kind{Type: &buildv1alpha1.Function{}}, controllers.EnqueueTracked(&buildv1alpha1.Function{}, r.Tracker, r.Scheme)).
		Complete(r)
}

func DeployerChildDeploymentReconciler(r *DeployerReconciler) controllers.SubReconciler {
	var cr *controllers.ChildReconciler
	cr = &controllers.ChildReconciler{
		Config: r.WithLog(r.Log.WithName("Deployment")),

		IndexField: deploymentIndexField,

		ParentType:    &corev1alpha1.Deployer{},
		ChildType:     &appsv1.Deployment{},
		ChildListType: &appsv1.DeploymentList{},

		SemanticEquals: func(a1, a2 *appsv1.Deployment) bool {
			return equality.Semantic.DeepEqual(a1.Spec, a2.Spec) &&
				equality.Semantic.DeepEqual(a1.Labels, a2.Labels)
		},
		DesiredChild: func(parent *corev1alpha1.Deployer) (*appsv1.Deployment, error) {
			labels := r.constructLabelsForDeployer(parent)

			template := *parent.Spec.Template.DeepCopy()
			targetPort := template.Spec.Containers[0].Ports[0]

			for k, v := range r.constructLabelsForDeployer(parent) {
				template.Labels[k] = v
			}

			template.Spec.Containers[0].Env = append(template.Spec.Containers[0].Env, corev1.EnvVar{
				Name:  "PORT",
				Value: fmt.Sprintf("%d", targetPort.ContainerPort),
			})
			if template.Spec.Containers[0].ReadinessProbe == nil {
				template.Spec.Containers[0].ReadinessProbe = &corev1.Probe{
					Handler: corev1.Handler{
						TCPSocket: &corev1.TCPSocketAction{
							Port: intstr.FromInt(int(targetPort.ContainerPort)),
						},
					},
				}
			}

			child := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Labels:       labels,
					Annotations:  make(map[string]string),
					GenerateName: fmt.Sprintf("%s-deployer-", parent.Name),
					Namespace:    parent.Namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							corev1alpha1.DeployerLabelKey: parent.Name,
						},
					},
					Template: template,
				},
			}
			if child.Spec.Template.Spec.Containers[0].Image == "" {
				child.Spec.Template.Spec.Containers[0].Image = parent.Status.LatestImage
			}

			return child, nil
		},
		ReflectChildStatusOnParent: func(parent *corev1alpha1.Deployer, child *appsv1.Deployment, err error) {
			if err != nil {
				return
			}
			parent.Status.DeploymentRef = refs.NewTypedLocalObjectReferenceForObject(child, r.Scheme)
			parent.Status.PropagateDeploymentStatus(&child.Status)
		},
		HarmonizeImmutableFields: func(current, desired *appsv1.Deployment) {
			desired.Spec.Replicas = current.Spec.Replicas
		},
		MergeBeforeUpdate: func(current, desired *appsv1.Deployment) {
			current.Labels = desired.Labels
			current.Spec = desired.Spec
		},
		Sanitize: func(child *appsv1.Deployment) interface{} {
			return child.Spec
		},
	}
	return cr
}

func DeployerChildServiceReconciler(r *DeployerReconciler) controllers.SubReconciler {
	var cr *controllers.ChildReconciler
	cr = &controllers.ChildReconciler{
		Config: r.WithLog(r.Log.WithName("Service")),

		IndexField: serviceIndexField,

		ParentType:    &corev1alpha1.Deployer{},
		ChildType:     &corev1.Service{},
		ChildListType: &corev1.ServiceList{},

		SemanticEquals: func(a1, a2 *corev1.Service) bool {
			return equality.Semantic.DeepEqual(a1.Spec, a2.Spec) &&
				equality.Semantic.DeepEqual(a1.Labels, a2.Labels)
		},
		DesiredChild: func(parent *corev1alpha1.Deployer) (*corev1.Service, error) {
			labels := r.constructLabelsForDeployer(parent)
			targetPort := parent.Spec.Template.Spec.Containers[0].Ports[0]

			child := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: make(map[string]string),
					Namespace:   parent.Namespace,
					Name:        parent.Name,
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: targetPort.Name, Port: 80, TargetPort: intstr.FromInt(int(targetPort.ContainerPort))},
					},
					Selector: map[string]string{
						corev1alpha1.DeployerLabelKey: parent.Name,
					},
				},
			}

			return child, nil
		},
		ReflectChildStatusOnParent: func(parent *corev1alpha1.Deployer, child *corev1.Service, err error) {
			if err != nil {
				if apierrs.IsAlreadyExists(err) {
					name := err.(apierrs.APIStatus).Status().Details.Name
					parent.Status.MarkServiceNotOwned(name)
				}
				return
			}
			parent.Status.ServiceRef = refs.NewTypedLocalObjectReferenceForObject(child, r.Scheme)
			parent.Status.Address = &apis.Addressable{URL: fmt.Sprintf("http://%s.%s.%s", child.Name, child.Namespace, "svc.cluster.local")}
			parent.Status.PropagateServiceStatus(&child.Status)
		},
		HarmonizeImmutableFields: func(current, desired *corev1.Service) {
			desired.Spec.ClusterIP = current.Spec.ClusterIP
		},
		MergeBeforeUpdate: func(current, desired *corev1.Service) {
			current.Labels = desired.Labels
			current.Spec = desired.Spec
		},
		Sanitize: func(child *corev1.Service) interface{} {
			return child.Spec
		},
	}
	return cr
}

func NewDeployerChildIngressReconciler(r *DeployerReconciler) controllers.SubReconciler {
	var cr *controllers.ChildReconciler
	cr = &controllers.ChildReconciler{
		Config: r.WithLog(r.Log.WithName("Ingress")),

		IndexField: ingressIndexField,

		ParentType:    &corev1alpha1.Deployer{},
		ChildType:     &networkingv1beta1.Ingress{},
		ChildListType: &networkingv1beta1.IngressList{},

		SemanticEquals: func(a1, a2 *networkingv1beta1.Ingress) bool {
			return equality.Semantic.DeepEqual(a1.Spec, a2.Spec) &&
				equality.Semantic.DeepEqual(a1.Labels, a2.Labels)
		},
		DesiredChild: func(parent *corev1alpha1.Deployer) (*networkingv1beta1.Ingress, error) {
			if parent.Status.ServiceRef == nil || parent.Spec.IngressPolicy == corev1alpha1.IngressPolicyClusterLocal {
				// skip ingress
				return nil, nil
			}

			coreSettings := &corev1.ConfigMap{}
			coreSettingsKey := types.NamespacedName{Namespace: systemNamespace, Name: settingsConfigMapName}

			// track config map
			r.Tracker.Track(
				tracker.NewKey(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, coreSettingsKey),
				types.NamespacedName{Namespace: parent.Namespace, Name: parent.Name},
			)
			if err := r.Get(context.TODO(), coreSettingsKey, coreSettings); err != nil {
				r.Log.Error(err, fmt.Sprintf("unable to fetch resource with reference: %s", coreSettingsKey.String()))
				return nil, err
			}

			domain := defaultDomain
			if d := coreSettings.Data[defaultDomainKey]; d != "" {
				domain = d
			}
			host := fmt.Sprintf("%s.%s.%s", parent.Name, parent.Namespace, domain)

			child := &networkingv1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Labels:       r.constructLabelsForDeployer(parent),
					Annotations:  make(map[string]string),
					GenerateName: fmt.Sprintf("%s-deployer-", parent.Name),
					Namespace:    parent.Namespace,
				},
				Spec: networkingv1beta1.IngressSpec{
					Rules: []networkingv1beta1.IngressRule{{
						Host: host,
						IngressRuleValue: networkingv1beta1.IngressRuleValue{
							HTTP: &networkingv1beta1.HTTPIngressRuleValue{
								Paths: []networkingv1beta1.HTTPIngressPath{{
									Path: "/",
									Backend: networkingv1beta1.IngressBackend{
										ServiceName: parent.Status.ServiceRef.Name,
										ServicePort: intstr.FromInt(80),
									},
								}},
							},
						},
					}},
				},
			}

			return child, nil
		},
		ReflectChildStatusOnParent: func(parent *corev1alpha1.Deployer, child *networkingv1beta1.Ingress, err error) {
			if err != nil {
				return
			}
			if child == nil {
				parent.Status.IngressRef = nil
				parent.Status.URL = ""
				parent.Status.MarkIngressNotRequired()
			} else {
				parent.Status.IngressRef = refs.NewTypedLocalObjectReferenceForObject(child, r.Scheme)
				parent.Status.URL = fmt.Sprintf("http://%s", child.Spec.Rules[0].Host)
				parent.Status.PropagateIngressStatus(&child.Status)
			}
		},
		MergeBeforeUpdate: func(current, desired *networkingv1beta1.Ingress) {
			current.Labels = desired.Labels
			current.Spec = desired.Spec
		},
		Sanitize: func(child *networkingv1beta1.Ingress) interface{} {
			return child.Spec
		},
	}
	return cr
}
