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

	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	knativev1alpha1 "github.com/projectriff/system/pkg/apis/knative/v1alpha1"
	servingv1 "github.com/projectriff/system/pkg/apis/thirdparty/knative/serving/v1"
	"github.com/projectriff/system/pkg/controllers"
	"github.com/projectriff/system/pkg/refs"
	"github.com/projectriff/system/pkg/tracker"
)

// +kubebuilder:rbac:groups=knative.projectriff.io,resources=deployers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=knative.projectriff.io,resources=deployers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=build.projectriff.io,resources=applications;containers;functions,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.knative.dev,resources=configurations;routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

func DeployerReconciler(config controllers.Config) *controllers.ParentReconciler {
	return &controllers.ParentReconciler{
		Config:     config,
		ParentType: &knativev1alpha1.Deployer{},
		SubReconcilers: []controllers.SubReconciler{
			DeployerBuildReconciler(config),
			DeployerChildConfigurationReconciler(config),
			DeployerChildRouteReconciler(config),
		},
	}
}

func DeployerBuildReconciler(config controllers.Config) controllers.SubReconciler {
	var r *controllers.SyncReconciler

	resolveBuild := func(ctx context.Context, parent *knativev1alpha1.Deployer) error {
		build := parent.Spec.Build
		if build == nil {
			parent.Status.LatestImage = parent.Spec.Template.Spec.Containers[0].Image
			return nil
		}

		switch {
		case build.ApplicationRef != "":
			var application buildv1alpha1.Application
			key := types.NamespacedName{Namespace: parent.Namespace, Name: build.ApplicationRef}
			// track application for new images
			r.Tracker.Track(
				tracker.NewKey(application.GetGroupVersionKind(), key),
				types.NamespacedName{Namespace: parent.Namespace, Name: parent.Name},
			)
			if err := r.Get(ctx, key, &application); err != nil {
				return err
			}
			if application.Status.LatestImage != "" {
				parent.Status.LatestImage = application.Status.LatestImage
			}
			return nil

		case build.ContainerRef != "":
			var container buildv1alpha1.Container
			key := types.NamespacedName{Namespace: parent.Namespace, Name: build.ContainerRef}
			// track container for new images
			r.Tracker.Track(
				tracker.NewKey(container.GetGroupVersionKind(), key),
				types.NamespacedName{Namespace: parent.Namespace, Name: parent.Name},
			)
			if err := r.Get(ctx, key, &container); err != nil {
				return err
			}
			if container.Status.LatestImage != "" {
				parent.Status.LatestImage = container.Status.LatestImage
			}
			return nil

		case build.FunctionRef != "":
			var function buildv1alpha1.Function
			key := types.NamespacedName{Namespace: parent.Namespace, Name: build.FunctionRef}
			// track function for new images
			r.Tracker.Track(
				tracker.NewKey(function.GetGroupVersionKind(), key),
				types.NamespacedName{Namespace: parent.Namespace, Name: parent.Name},
			)
			if err := r.Get(ctx, key, &function); err != nil {
				return err
			}
			if function.Status.LatestImage != "" {
				parent.Status.LatestImage = function.Status.LatestImage
			}
			return nil

		}

		return fmt.Errorf("invalid build")
	}

	r = &controllers.SyncReconciler{
		Config:     config.WithLog(config.Log.WithName("Build")),
		ParentType: &knativev1alpha1.Deployer{},

		Setup: func(mgr manager.Manager, bldr *builder.Builder) error {
			bldr.Watches(&source.Kind{Type: &buildv1alpha1.Application{}}, controllers.EnqueueTracked(&buildv1alpha1.Application{}, r.Tracker, r.Scheme))
			bldr.Watches(&source.Kind{Type: &buildv1alpha1.Container{}}, controllers.EnqueueTracked(&buildv1alpha1.Container{}, r.Tracker, r.Scheme))
			bldr.Watches(&source.Kind{Type: &buildv1alpha1.Function{}}, controllers.EnqueueTracked(&buildv1alpha1.Function{}, r.Tracker, r.Scheme))
			return nil
		},

		Sync: func(ctx context.Context, parent *knativev1alpha1.Deployer) error {
			if err := resolveBuild(ctx, parent); err != nil {
				if apierrs.IsNotFound(err) {
					// we'll ignore not-found errors, since the reference build resource
					// may not exist yet.
					return nil
				}
				r.Log.Error(err, "unable to resolve image for Deployer", "deployer", parent)
				return err
			}
			return nil
		},
	}
	return r
}

func DeployerChildConfigurationReconciler(config controllers.Config) controllers.SubReconciler {
	var r *controllers.ChildReconciler
	r = &controllers.ChildReconciler{
		Config: config.WithLog(config.Log.WithName("Configuration")),

		IndexField: ".metadata.configurationController",

		ParentType:    &knativev1alpha1.Deployer{},
		ChildType:     &servingv1.Configuration{},
		ChildListType: &servingv1.ConfigurationList{},

		SemanticEquals: func(a1, a2 *servingv1.Configuration) bool {
			return equality.Semantic.DeepEqual(a1.Spec, a2.Spec) &&
				equality.Semantic.DeepEqual(a1.Labels, a2.Labels) &&
				equality.Semantic.DeepEqual(a1.Annotations, a2.Annotations)
		},
		DesiredChild: func(parent *knativev1alpha1.Deployer) (*servingv1.Configuration, error) {
			if parent.Status.LatestImage == "" {
				return nil, nil
			}

			labels := controllers.MergeMaps(parent.Labels, map[string]string{
				knativev1alpha1.DeployerLabelKey: parent.Name,
			})
			annotations := map[string]string{}
			if parent.Spec.Scale.Min != nil {
				annotations["autoscaling.knative.dev/minScale"] = fmt.Sprintf("%d", *parent.Spec.Scale.Min)
			}
			if parent.Spec.Scale.Max != nil {
				annotations["autoscaling.knative.dev/maxScale"] = fmt.Sprintf("%d", *parent.Spec.Scale.Max)
			}

			template := parent.Spec.Template.DeepCopy()
			template.Annotations = controllers.MergeMaps(annotations, template.Annotations)
			template.Labels = controllers.MergeMaps(labels, template.Labels)

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

			return child, nil
		},
		ReflectChildStatusOnParent: func(parent *knativev1alpha1.Deployer, child *servingv1.Configuration, err error) {
			if child == nil {
				parent.Status.ConfigurationRef = nil
			} else {
				parent.Status.ConfigurationRef = refs.NewTypedLocalObjectReferenceForObject(child, r.Scheme)
				parent.Status.PropagateConfigurationStatus(&child.Status)
			}
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
	return r
}

func DeployerChildRouteReconciler(config controllers.Config) controllers.SubReconciler {
	var r *controllers.ChildReconciler
	r = &controllers.ChildReconciler{
		Config: config.WithLog(config.Log.WithName("Route")),

		IndexField: ".metadata.routeController",

		ParentType:    &knativev1alpha1.Deployer{},
		ChildType:     &servingv1.Route{},
		ChildListType: &servingv1.RouteList{},

		SemanticEquals: func(a1, a2 *servingv1.Route) bool {
			return equality.Semantic.DeepEqual(a1.Spec, a2.Spec) &&
				equality.Semantic.DeepEqual(a1.Labels, a2.Labels)
		},
		DesiredChild: func(parent *knativev1alpha1.Deployer) (*servingv1.Route, error) {
			if parent.Status.ConfigurationRef == nil {
				return nil, nil
			}

			labels := controllers.MergeMaps(parent.Labels, map[string]string{
				knativev1alpha1.DeployerLabelKey: parent.Name,
			})
			if parent.Spec.IngressPolicy == knativev1alpha1.IngressPolicyClusterLocal {
				labels["serving.knative.dev/visibility"] = "cluster-local"
			}
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
			if child == nil {
				parent.Status.RouteRef = nil
			} else {
				parent.Status.RouteRef = refs.NewTypedLocalObjectReferenceForObject(child, r.Scheme)
				parent.Status.PropagateRouteStatus(&child.Status)
			}
		},
		MergeBeforeUpdate: func(current, desired *servingv1.Route) {
			current.Labels = desired.Labels
			current.Spec = desired.Spec
		},
		Sanitize: func(child *servingv1.Route) interface{} {
			return child.Spec
		},
	}
	return r
}
