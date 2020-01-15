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

package knative

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/source"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	knativev1alpha1 "github.com/projectriff/system/pkg/apis/knative/v1alpha1"
	servingv1 "github.com/projectriff/system/pkg/apis/thirdparty/knative/serving/v1"
	"github.com/projectriff/system/pkg/controllers"
	"github.com/projectriff/system/pkg/refs"
	"github.com/projectriff/system/pkg/tracker"
)

const (
	configurationIndexField = ".metadata.configurationController"
	routeIndexField         = ".metadata.routeController"
)

// DeployerReconciler reconciles a Deployer object
type DeployerReconciler struct {
	controllers.Config

	configurationReconciler controllers.SubReconciler
	routeReconciler         controllers.SubReconciler
}

// +kubebuilder:rbac:groups=knative.projectriff.io,resources=deployers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=knative.projectriff.io,resources=deployers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=build.projectriff.io,resources=applications;containers;functions,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.knative.dev,resources=configurations;routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

func (r *DeployerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("deployer", req.NamespacedName)

	// TODO move to an init or constructor
	if r.configurationReconciler == nil {
		r.configurationReconciler = DeployerChildConfigurationReconciler(r)
	}
	if r.routeReconciler == nil {
		r.routeReconciler = DeployerChildRouteReconciler(r)
	}

	var originalDeployer knativev1alpha1.Deployer
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

func (r *DeployerReconciler) reconcile(ctx context.Context, log logr.Logger, deployer *knativev1alpha1.Deployer) (ctrl.Result, error) {
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

	// reconcile configuration
	if _, err := r.configurationReconciler.Reconcile(ctx, deployer); err != nil {
		return ctrl.Result{}, err
	}

	// reconcile route
	if _, err := r.routeReconciler.Reconcile(ctx, deployer); err != nil {
		return ctrl.Result{}, err
	}

	deployer.Status.ObservedGeneration = deployer.Generation

	return ctrl.Result{}, nil
}

func (r *DeployerReconciler) reconcileBuildImage(ctx context.Context, log logr.Logger, deployer *knativev1alpha1.Deployer) error {
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
			// TODO this should not be an error
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
			// TODO this should not be an error
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
			// TODO this should not be an error
			return fmt.Errorf("function %q does not have a ready image", build.FunctionRef)
		}
		deployer.Status.LatestImage = function.Status.LatestImage
		return nil

	}

	return fmt.Errorf("invalid deployer build")
}

func (r *DeployerReconciler) constructLabelsForDeployer(deployer *knativev1alpha1.Deployer) map[string]string {
	labels := make(map[string]string, len(deployer.ObjectMeta.Labels)+2)
	// pass through existing labels
	for k, v := range deployer.ObjectMeta.Labels {
		labels[k] = v
	}

	labels[knativev1alpha1.DeployerLabelKey] = deployer.Name

	if deployer.Spec.IngressPolicy == knativev1alpha1.IngressPolicyClusterLocal {
		labels["serving.knative.dev/visibility"] = "cluster-local"
	}

	return labels
}

func (r *DeployerReconciler) constructAnnotationsForDeployer(deployer *knativev1alpha1.Deployer) map[string]string {
	// make a little extra space just in case
	annotations := make(map[string]string, len(deployer.ObjectMeta.Annotations)+2)

	// pass through existing annotations
	for k, v := range deployer.ObjectMeta.Annotations {
		annotations[k] = v
	}

	if deployer.Spec.Scale.Min != nil {
		annotations["autoscaling.knative.dev/minScale"] = fmt.Sprintf("%d", *deployer.Spec.Scale.Min)
	}
	if deployer.Spec.Scale.Max != nil {
		annotations["autoscaling.knative.dev/maxScale"] = fmt.Sprintf("%d", *deployer.Spec.Scale.Max)
	}

	return annotations
}

func (r *DeployerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := controllers.IndexControllersOfType(mgr, configurationIndexField, &knativev1alpha1.Deployer{}, &servingv1.Configuration{}); err != nil {
		return err
	}
	if err := controllers.IndexControllersOfType(mgr, routeIndexField, &knativev1alpha1.Deployer{}, &servingv1.Route{}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&knativev1alpha1.Deployer{}).
		Owns(&servingv1.Configuration{}).
		Owns(&servingv1.Route{}).
		// watch for build mutations to update dependent deployers
		Watches(&source.Kind{Type: &buildv1alpha1.Application{}}, controllers.EnqueueTracked(&buildv1alpha1.Application{}, r.Tracker, r.Scheme)).
		Watches(&source.Kind{Type: &buildv1alpha1.Container{}}, controllers.EnqueueTracked(&buildv1alpha1.Container{}, r.Tracker, r.Scheme)).
		Watches(&source.Kind{Type: &buildv1alpha1.Function{}}, controllers.EnqueueTracked(&buildv1alpha1.Function{}, r.Tracker, r.Scheme)).
		Complete(r)
}

func DeployerChildConfigurationReconciler(r *DeployerReconciler) controllers.SubReconciler {
	var cr *controllers.ChildReconciler
	cr = &controllers.ChildReconciler{
		Config: r.Config,

		IndexField: configurationIndexField,

		ParentType:    reflect.TypeOf(knativev1alpha1.Deployer{}),
		ChildType:     reflect.TypeOf(servingv1.Configuration{}),
		ChildListType: reflect.TypeOf(servingv1.ConfigurationList{}),

		SemanticEquals: func(a1, a2 *servingv1.Configuration) bool {
			return equality.Semantic.DeepEqual(a1.Spec, a2.Spec) &&
				equality.Semantic.DeepEqual(a1.Labels, a2.Labels) &&
				equality.Semantic.DeepEqual(a1.Annotations, a2.Annotations)
		},
		ConstructDesiredChild: func(parent *knativev1alpha1.Deployer) (*servingv1.Configuration, error) {
			labels := r.constructLabelsForDeployer(parent)
			annotations := r.constructAnnotationsForDeployer(parent)
			template := parent.Spec.Template.DeepCopy()

			for k, v := range labels {
				template.Labels[k] = v
			}
			for k, v := range annotations {
				template.Annotations[k] = v
			}

			child := &servingv1.Configuration{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: fmt.Sprintf("%s-deployer-", parent.Name),
					Namespace:    parent.Namespace,
					Labels:       labels,
					Annotations:  annotations,
				},
				Spec: servingv1.ConfigurationSpec{
					Template: servingv1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      labels,
							Annotations: annotations,
						},
						Spec: servingv1.RevisionSpec{
							PodSpec: template.Spec,
						},
					},
				},
			}
			if child.Spec.Template.Spec.Containers[0].Name == "" {
				child.Spec.Template.Spec.Containers[0].Name = "user-container"
			}
			if child.Spec.Template.Spec.Containers[0].Image == "" {
				child.Spec.Template.Spec.Containers[0].Image = parent.Status.LatestImage
			}
			if err := ctrl.SetControllerReference(parent, child, r.Scheme); err != nil {
				return nil, err
			}

			return child, nil
		},
		ReflectChildStatusOnParent: func(parent *knativev1alpha1.Deployer, child *servingv1.Configuration, err error) {
			parent.Status.ConfigurationRef = refs.NewTypedLocalObjectReferenceForObject(child, r.Scheme)
			parent.Status.PropagateConfigurationStatus(&child.Status)
		},
		MergeBeforeUpdate: func(current, desired *servingv1.Configuration) {
			current.Labels = desired.Labels
			current.Annotations = desired.Annotations
			current.Spec = desired.Spec
		},
		Sanitize: func(child *servingv1.Configuration) interface{} {
			return child.Spec
		},
	}
	cr.Log = r.Log.WithName(cr.ChildType.Name())
	return cr
}

func DeployerChildRouteReconciler(r *DeployerReconciler) controllers.SubReconciler {
	var cr *controllers.ChildReconciler
	cr = &controllers.ChildReconciler{
		Config: r.Config,

		IndexField: routeIndexField,

		ParentType:    reflect.TypeOf(knativev1alpha1.Deployer{}),
		ChildType:     reflect.TypeOf(servingv1.Route{}),
		ChildListType: reflect.TypeOf(servingv1.RouteList{}),

		SemanticEquals: func(a1, a2 *servingv1.Route) bool {
			return equality.Semantic.DeepEqual(a1.Spec, a2.Spec) &&
				equality.Semantic.DeepEqual(a1.Labels, a2.Labels)
		},
		ConstructDesiredChild: func(parent *knativev1alpha1.Deployer) (*servingv1.Route, error) {
			if parent.Status.ConfigurationRef == nil {
				return nil, fmt.Errorf("unable to create Route, waiting for Configuration")
			}

			labels := r.constructLabelsForDeployer(parent)
			var allTraffic int64 = 100

			child := &servingv1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: make(map[string]string),
					Namespace:   parent.Namespace,
					Name:        parent.Name,
				},
				Spec: servingv1.RouteSpec{
					Traffic: []servingv1.TrafficTarget{
						{
							Percent:           &allTraffic,
							ConfigurationName: parent.Status.ConfigurationRef.Name,
						},
					},
				},
			}
			if err := ctrl.SetControllerReference(parent, child, r.Scheme); err != nil {
				return nil, err
			}

			return child, nil
		},
		ReflectChildStatusOnParent: func(parent *knativev1alpha1.Deployer, child *servingv1.Route, err error) {
			if err != nil {
				if apierrs.IsAlreadyExists(err) {
					name := err.(apierrs.APIStatus).Status().Details.Name
					parent.Status.MarkRouteNotOwned(name)
				}
				return
			}
			parent.Status.RouteRef = refs.NewTypedLocalObjectReferenceForObject(child, r.Scheme)
			parent.Status.PropagateRouteStatus(&child.Status)
		},
		MergeBeforeUpdate: func(current, desired *servingv1.Route) {
			current.Labels = desired.Labels
			current.Spec = desired.Spec
		},
		Sanitize: func(child *servingv1.Route) interface{} {
			return child.Spec
		},
	}
	cr.Log = r.Log.WithName(cr.ChildType.Name())
	return cr
}
