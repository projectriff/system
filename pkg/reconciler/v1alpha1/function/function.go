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

package function

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/kmp"
	"github.com/knative/pkg/logging"
	projectriffv1alpha1 "github.com/projectriff/system/pkg/apis/projectriff/v1alpha1"
	projectriffinformers "github.com/projectriff/system/pkg/client/informers/externalversions/projectriff/v1alpha1"
	projectrifflisters "github.com/projectriff/system/pkg/client/listers/projectriff/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/function/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/function/resources/names"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	// ReconcilerName is the name of the reconciler
	ReconcilerName      = "Functions"
	controllerAgentName = "function-controller"
)

// Reconciler implements controller.Reconciler for Function resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	functionLister    projectrifflisters.FunctionLister
	applicationLister projectrifflisters.ApplicationLister
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	functionInformer projectriffinformers.FunctionInformer,
	applicationInformer projectriffinformers.ApplicationInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:              reconciler.NewBase(opt, controllerAgentName),
		functionLister:    functionInformer.Lister(),
		applicationLister: applicationInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	functionInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
		DeleteFunc: impl.Enqueue,
	})

	applicationInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(projectriffv1alpha1.SchemeGroupVersion.WithKind("Function")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Function resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the Function resource with this namespace/name
	original, err := c.functionLister.Functions(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("function %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	function := original.DeepCopy()

	// Reconcile this copy of the function and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, function)

	if equality.Semantic.DeepEqual(original.Status, function.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(function); uErr != nil {
		logger.Warn("Failed to update function status", zap.Error(uErr))
		c.Recorder.Eventf(function, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for Function %q: %v", function.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(function, corev1.EventTypeNormal, "Updated", "Updated Function %q", function.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, function *projectriffv1alpha1.Function) error {
	logger := logging.FromContext(ctx)
	if function.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	function.SetDefaults()

	function.Status.InitializeConditions()

	applicationName := resourcenames.Application(function)
	application, err := c.applicationLister.Applications(function.Namespace).Get(applicationName)
	if errors.IsNotFound(err) {
		application, err = c.createApplication(function)
		if err != nil {
			logger.Errorf("Failed to create Application %q: %v", applicationName, err)
			c.Recorder.Eventf(function, corev1.EventTypeWarning, "CreationFailed", "Failed to create Application %q: %v", applicationName, err)
			return err
		}
		c.Recorder.Eventf(function, corev1.EventTypeNormal, "Created", "Created Application %q", applicationName)
	} else if err != nil {
		logger.Errorf("Failed to reconcile Function: %q failed to Get Application: %q; %v", function.Name, applicationName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(application, function) {
		// Surface an error in the function's status,and return an error.
		function.Status.MarkApplicationNotOwned(applicationName)
		return fmt.Errorf("Function: %q does not own Application: %q", function.Name, applicationName)
	} else if application, err = c.reconcileApplication(ctx, function, application); err != nil {
		logger.Errorf("Failed to reconcile Function: %q failed to reconcile Application: %q; %v", function.Name, applicationName, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Application.
	function.Status.PropagateApplicationStatus(&application.Status)

	function.Status.ObservedGeneration = function.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *projectriffv1alpha1.Function) (*projectriffv1alpha1.Function, error) {
	function, err := c.functionLister.Functions(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(function.Status, desired.Status) {
		return function, nil
	}
	becomesReady := desired.Status.IsReady() && !function.Status.IsReady()
	// Don't modify the informers copy.
	existing := function.DeepCopy()
	existing.Status = desired.Status

	fn, err := c.ProjectriffClientSet.ProjectriffV1alpha1().Functions(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(fn.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("Function %q became ready after %v", function.Name, duration)
	}

	return fn, err
}

func (c *Reconciler) reconcileApplication(ctx context.Context, function *projectriffv1alpha1.Function, application *projectriffv1alpha1.Application) (*projectriffv1alpha1.Application, error) {
	logger := logging.FromContext(ctx)
	desiredApplication, err := resources.MakeApplication(function)
	if err != nil {
		return nil, err
	}

	if applicationSemanticEquals(desiredApplication, application) {
		// No differences to reconcile.
		return application, nil
	}
	diff, err := kmp.SafeDiff(desiredApplication.Spec, application.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff Application: %v", err)
	}
	logger.Infof("Reconciling application diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := application.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredApplication.Spec
	existing.ObjectMeta.Labels = desiredApplication.ObjectMeta.Labels
	return c.ProjectriffClientSet.ProjectriffV1alpha1().Applications(function.Namespace).Update(existing)
}

func (c *Reconciler) createApplication(function *projectriffv1alpha1.Function) (*projectriffv1alpha1.Application, error) {
	cfg, err := resources.MakeApplication(function)
	if err != nil {
		return nil, err
	}
	return c.ProjectriffClientSet.ProjectriffV1alpha1().Applications(function.Namespace).Create(cfg)
}

func applicationSemanticEquals(desiredApplication, application *projectriffv1alpha1.Application) bool {
	return equality.Semantic.DeepEqual(desiredApplication.Spec, application.Spec) &&
		equality.Semantic.DeepEqual(desiredApplication.ObjectMeta.Labels, application.ObjectMeta.Labels)
}
