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
	planv1alpha1 "github.com/projectriff/system/pkg/apis/plan/v1alpha1"
	projectriffv1alpha1 "github.com/projectriff/system/pkg/apis/projectriff/v1alpha1"
	planinformers "github.com/projectriff/system/pkg/client/informers/externalversions/plan/v1alpha1"
	projectriffinformers "github.com/projectriff/system/pkg/client/informers/externalversions/projectriff/v1alpha1"
	planlisters "github.com/projectriff/system/pkg/client/listers/plan/v1alpha1"
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
	functionLister projectrifflisters.FunctionLister
	planLister     planlisters.PlanLister
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	functionInformer projectriffinformers.FunctionInformer,
	planInformer planinformers.PlanInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:           reconciler.NewBase(opt, controllerAgentName),
		functionLister: functionInformer.Lister(),
		planLister:     planInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	functionInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
		DeleteFunc: impl.Enqueue,
	})

	planInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
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
	function.SetDefaults(context.Background())

	function.Status.InitializeConditions()

	planName := resourcenames.Plan(function)
	plan, err := c.planLister.Plans(function.Namespace).Get(planName)
	if errors.IsNotFound(err) {
		plan, err = c.createPlan(function)
		if err != nil {
			logger.Errorf("Failed to create Plan %q: %v", planName, err)
			c.Recorder.Eventf(function, corev1.EventTypeWarning, "CreationFailed", "Failed to create Plan %q: %v", planName, err)
			return err
		}
		c.Recorder.Eventf(function, corev1.EventTypeNormal, "Created", "Created Plan %q", planName)
	} else if err != nil {
		logger.Errorf("Failed to reconcile Function: %q failed to Get Plan: %q; %v", function.Name, planName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(plan, function) {
		// Surface an error in the function's status,and return an error.
		function.Status.MarkPlanNotOwned(planName)
		return fmt.Errorf("Function: %q does not own Plan: %q", function.Name, planName)
	} else if plan, err = c.reconcilePlan(ctx, function, plan); err != nil {
		logger.Errorf("Failed to reconcile Function: %q failed to reconcile Plan: %q; %v", function.Name, planName, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Plan.
	function.Status.PropagatePlanStatus(plan)

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

func (c *Reconciler) reconcilePlan(ctx context.Context, function *projectriffv1alpha1.Function, plan *planv1alpha1.Plan) (*planv1alpha1.Plan, error) {
	logger := logging.FromContext(ctx)
	desiredPlan, err := resources.MakePlan(function)
	if err != nil {
		return nil, err
	}

	if planSemanticEquals(desiredPlan, plan) {
		// No differences to reconcile.
		return plan, nil
	}
	diff, err := kmp.SafeDiff(desiredPlan.Spec, plan.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff Plan: %v", err)
	}
	logger.Infof("Reconciling plan diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := plan.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredPlan.Spec
	existing.ObjectMeta.Labels = desiredPlan.ObjectMeta.Labels
	return c.ProjectriffClientSet.PlanV1alpha1().Plans(function.Namespace).Update(existing)
}

func (c *Reconciler) createPlan(function *projectriffv1alpha1.Function) (*planv1alpha1.Plan, error) {
	plan, err := resources.MakePlan(function)
	if err != nil {
		return nil, err
	}
	return c.ProjectriffClientSet.PlanV1alpha1().Plans(function.Namespace).Create(plan)
}

func planSemanticEquals(desiredPlan, plan *planv1alpha1.Plan) bool {
	return equality.Semantic.DeepEqual(desiredPlan.Spec, plan.Spec) &&
		equality.Semantic.DeepEqual(desiredPlan.ObjectMeta.Labels, plan.ObjectMeta.Labels)
}
