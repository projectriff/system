/*
Copyright 2018 The Knative Authors

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

package knativeadapter

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/logging"
	"github.com/knative/pkg/tracker"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	knservingv1beta1 "github.com/knative/serving/pkg/apis/serving/v1beta1"
	knservinginformers "github.com/knative/serving/pkg/client/informers/externalversions/serving/v1alpha1"
	knservinglisters "github.com/knative/serving/pkg/client/listers/serving/v1alpha1"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	knativev1alpha1 "github.com/projectriff/system/pkg/apis/knative/v1alpha1"
	buildinformers "github.com/projectriff/system/pkg/client/informers/externalversions/build/v1alpha1"
	knativeinformers "github.com/projectriff/system/pkg/client/informers/externalversions/knative/v1alpha1"
	buildlisters "github.com/projectriff/system/pkg/client/listers/build/v1alpha1"
	knativelisters "github.com/projectriff/system/pkg/client/listers/knative/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
)

const (
	// ReconcilerName is the name of the reconciler
	ReconcilerName      = "Knative Adapters"
	controllerAgentName = "knative-adapter-controller"
)

// Reconciler implements controller.Reconciler for Adapter resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	adapterLister         knativelisters.AdapterLister
	applicationLister     buildlisters.ApplicationLister
	containerLister       buildlisters.ContainerLister
	functionLister        buildlisters.FunctionLister
	knconfigurationLister knservinglisters.ConfigurationLister
	knserviceLister       knservinglisters.ServiceLister

	tracker tracker.Interface
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	adapterInformer knativeinformers.AdapterInformer,
	applicationInformer buildinformers.ApplicationInformer,
	containerInformer buildinformers.ContainerInformer,
	functionInformer buildinformers.FunctionInformer,
	knserviceInformer knservinginformers.ServiceInformer,
	knconfigurationInformer knservinginformers.ConfigurationInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:                  reconciler.NewBase(opt, controllerAgentName),
		adapterLister:         adapterInformer.Lister(),
		applicationLister:     applicationInformer.Lister(),
		containerLister:       containerInformer.Lister(),
		functionLister:        functionInformer.Lister(),
		knserviceLister:       knserviceInformer.Lister(),
		knconfigurationLister: knconfigurationInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName)

	c.Logger.Info("Setting up event handlers")
	adapterInformer.Informer().AddEventHandler(reconciler.Handler(impl.Enqueue))

	// referenced resources
	c.tracker = tracker.New(impl.EnqueueKey, opt.GetTrackerLease())
	applicationInformer.Informer().AddEventHandler(reconciler.Handler(
		// Call the tracker's OnChanged method, but we've seen the objects
		// coming through this path missing TypeMeta, so ensure it is properly
		// populated.
		controller.EnsureTypeMeta(
			c.tracker.OnChanged,
			buildv1alpha1.SchemeGroupVersion.WithKind("Application"),
		),
	))
	containerInformer.Informer().AddEventHandler(reconciler.Handler(
		// Call the tracker's OnChanged method, but we've seen the objects
		// coming through this path missing TypeMeta, so ensure it is properly
		// populated.
		controller.EnsureTypeMeta(
			c.tracker.OnChanged,
			buildv1alpha1.SchemeGroupVersion.WithKind("Container"),
		),
	))
	functionInformer.Informer().AddEventHandler(reconciler.Handler(
		// Call the tracker's OnChanged method, but we've seen the objects
		// coming through this path missing TypeMeta, so ensure it is properly
		// populated.
		controller.EnsureTypeMeta(
			c.tracker.OnChanged,
			buildv1alpha1.SchemeGroupVersion.WithKind("Function"),
		),
	))

	knserviceInformer.Informer().AddEventHandler(reconciler.Handler(
		// Call the tracker's OnChanged method, but we've seen the objects
		// coming through this path missing TypeMeta, so ensure it is properly
		// populated.
		controller.EnsureTypeMeta(
			c.tracker.OnChanged,
			knservingv1alpha1.SchemeGroupVersion.WithKind("Service"),
		),
	))
	knconfigurationInformer.Informer().AddEventHandler(reconciler.Handler(
		// Call the tracker's OnChanged method, but we've seen the objects
		// coming through this path missing TypeMeta, so ensure it is properly
		// populated.
		controller.EnsureTypeMeta(
			c.tracker.OnChanged,
			knservingv1alpha1.SchemeGroupVersion.WithKind("Configuration"),
		),
	))

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Adapter resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the Adapter resource with this namespace/name
	original, err := c.adapterLister.Adapters(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("adapter %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	adapter := original.DeepCopy()

	// Reconcile this copy of the adapter and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, adapter)

	if equality.Semantic.DeepEqual(original.Status, adapter.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(adapter); uErr != nil {
		logger.Warn("Failed to update adapter status", zap.Error(uErr))
		c.Recorder.Eventf(adapter, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for Adapter %q: %v", adapter.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(adapter, corev1.EventTypeNormal, "Updated", "Updated Adapter %q", adapter.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, adapter *knativev1alpha1.Adapter) error {
	if adapter.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	adapter.SetDefaults(context.Background())

	adapter.Status.InitializeConditions()

	if err := c.reconcileBuild(adapter); err != nil {
		return err
	}

	if adapter.Status.LatestImage != "" {
		if err := c.reconcileTarget(ctx, adapter); err != nil {
			return err
		}
	}

	adapter.Status.ObservedGeneration = adapter.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *knativev1alpha1.Adapter) (*knativev1alpha1.Adapter, error) {
	adapter, err := c.adapterLister.Adapters(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(adapter.Status, desired.Status) {
		return adapter, nil
	}
	becomesReady := desired.Status.IsReady() && !adapter.Status.IsReady()
	// Don't modify the informers copy.
	existing := adapter.DeepCopy()
	existing.Status = desired.Status

	h, err := c.ProjectriffClientSet.KnativeV1alpha1().Adapters(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(h.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("Adapter %q became ready after %v", adapter.Name, duration)
	}

	return h, err
}

func (c *Reconciler) reconcileBuild(adapter *knativev1alpha1.Adapter) error {
	build := adapter.Spec.Build

	switch {
	case build.ApplicationRef != "":
		application, err := c.applicationLister.Applications(adapter.Namespace).Get(build.ApplicationRef)
		if err != nil {
			if errors.IsNotFound(err) {
				adapter.Status.MarkBuildNotFound("application", build.ApplicationRef)
				return nil
			}
			return err
		}
		if application.Status.LatestImage == "" {
			adapter.Status.MarkBuildLatestImageMissing("application", build.ApplicationRef)
			return nil
		}
		adapter.Status.LatestImage = application.Status.LatestImage
		adapter.Status.MarkBuildReady()

		// track application for new images
		gvk := buildv1alpha1.SchemeGroupVersion.WithKind("Application")
		if err := c.tracker.Track(reconciler.MakeObjectRef(application, gvk), adapter); err != nil {
			return err
		}
		return nil

	case build.ContainerRef != "":
		container, err := c.containerLister.Containers(adapter.Namespace).Get(build.ContainerRef)
		if err != nil {
			if errors.IsNotFound(err) {
				adapter.Status.MarkBuildNotFound("container", build.ContainerRef)
				return nil
			}
			return err
		}
		if container.Status.LatestImage == "" {
			adapter.Status.MarkBuildLatestImageMissing("container", build.ContainerRef)
			return nil
		}
		adapter.Status.LatestImage = container.Status.LatestImage
		adapter.Status.MarkBuildReady()

		// track function for new images
		gvk := buildv1alpha1.SchemeGroupVersion.WithKind("Container")
		if err := c.tracker.Track(reconciler.MakeObjectRef(container, gvk), adapter); err != nil {
			return err
		}
		return nil

	case build.FunctionRef != "":
		function, err := c.functionLister.Functions(adapter.Namespace).Get(build.FunctionRef)
		if err != nil {
			if errors.IsNotFound(err) {
				adapter.Status.MarkBuildNotFound("function", build.FunctionRef)
				return nil
			}
			return err
		}
		if function.Status.LatestImage == "" {
			adapter.Status.MarkBuildLatestImageMissing("function", build.FunctionRef)
			return nil
		}
		adapter.Status.LatestImage = function.Status.LatestImage
		adapter.Status.MarkBuildReady()

		// track function for new images
		gvk := buildv1alpha1.SchemeGroupVersion.WithKind("Function")
		if err := c.tracker.Track(reconciler.MakeObjectRef(function, gvk), adapter); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("invalid adapter build")
}

func (c *Reconciler) reconcileTarget(ctx context.Context, adapter *knativev1alpha1.Adapter) error {
	target := adapter.Spec.Target

	switch {
	case target.ServiceRef != "":
		service, err := c.knserviceLister.Services(adapter.Namespace).Get(target.ServiceRef)
		if err != nil {
			if errors.IsNotFound(err) {
				adapter.Status.MarkTargetNotFound("service", target.ServiceRef)
				return nil
			}
			return err
		}
		if err := service.ConvertUp(ctx, &knservingv1beta1.Service{}); err != nil {
			if ce, ok := err.(*knservingv1alpha1.CannotConvertError); ok {
				adapter.Status.MarkTargetInvalid("service", target.ServiceRef, ce)
				return nil
			}
			return err
		}
		adapter.Status.MarkTargetFound()

		if service.Spec.Template.Spec.Containers[0].Image == adapter.Status.LatestImage {
			// already latest image
			return nil
		}

		// update service
		service = service.DeepCopy()
		service.Spec.Template.Spec.Containers[0].Image = adapter.Status.LatestImage
		service, err = c.KnServingClientSet.ServingV1alpha1().Services(adapter.Namespace).Update(service)
		if err != nil {
			return err
		}

		// track service for changes
		gvk := knservingv1alpha1.SchemeGroupVersion.WithKind("Service")
		if err := c.tracker.Track(reconciler.MakeObjectRef(service, gvk), adapter); err != nil {
			return err
		}
		return nil

	case target.ConfigurationRef != "":
		configuration, err := c.knconfigurationLister.Configurations(adapter.Namespace).Get(target.ServiceRef)
		if err != nil {
			if errors.IsNotFound(err) {
				adapter.Status.MarkTargetNotFound("configuration", target.ConfigurationRef)
				return nil
			}
			return err
		}
		if err := configuration.ConvertUp(ctx, &knservingv1beta1.Configuration{}); err != nil {
			if ce, ok := err.(*knservingv1alpha1.CannotConvertError); ok {
				adapter.Status.MarkTargetInvalid("configuration", target.ServiceRef, ce)
				return nil
			}
			return err
		}
		adapter.Status.MarkTargetFound()

		if configuration.Spec.Template.Spec.Containers[0].Image == adapter.Status.LatestImage {
			// already latest image
			return nil
		}

		// update configuration
		configuration = configuration.DeepCopy()
		configuration.Spec.Template.Spec.Containers[0].Image = adapter.Status.LatestImage
		configuration, err = c.KnServingClientSet.ServingV1alpha1().Configurations(adapter.Namespace).Update(configuration)
		if err != nil {
			return err
		}

		// track configuration for changes
		gvk := knservingv1alpha1.SchemeGroupVersion.WithKind("Configuration")
		if err := c.tracker.Track(reconciler.MakeObjectRef(configuration, gvk), adapter); err != nil {
			return err
		}
	}

	return fmt.Errorf("invalid adapter target")
}
