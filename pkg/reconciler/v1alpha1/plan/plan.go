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

package plan

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/kmp"
	"github.com/knative/pkg/logging"
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	servinginformers "github.com/knative/serving/pkg/client/informers/externalversions/serving/v1alpha1"
	servinglisters "github.com/knative/serving/pkg/client/listers/serving/v1alpha1"
	planv1alpha1 "github.com/projectriff/system/pkg/apis/plan/v1alpha1"
	planinformers "github.com/projectriff/system/pkg/client/informers/externalversions/plan/v1alpha1"
	planlisters "github.com/projectriff/system/pkg/client/listers/plan/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/plan/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/plan/resources/names"
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
	ReconcilerName      = "Plans"
	controllerAgentName = "plan-controller"
)

// Reconciler implements controller.Reconciler for Plan resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	planLister      planlisters.PlanLister
	buildplanLister planlisters.BuildPlanLister
	runplanLister   planlisters.RunPlanLister
	routeLister     servinglisters.RouteLister
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	planInformer planinformers.PlanInformer,
	buildplanInformer planinformers.BuildPlanInformer,
	runplanInformer planinformers.RunPlanInformer,
	routeInformer servinginformers.RouteInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:            reconciler.NewBase(opt, controllerAgentName),
		planLister:      planInformer.Lister(),
		buildplanLister: buildplanInformer.Lister(),
		runplanLister:   runplanInformer.Lister(),
		routeLister:     routeInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	planInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
		DeleteFunc: impl.Enqueue,
	})

	buildplanInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(planv1alpha1.SchemeGroupVersion.WithKind("Plan")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})
	runplanInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(planv1alpha1.SchemeGroupVersion.WithKind("Plan")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})
	routeInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(planv1alpha1.SchemeGroupVersion.WithKind("Plan")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Plan resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the Plan resource with this namespace/name
	original, err := c.planLister.Plans(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("plan %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	plan := original.DeepCopy()

	// Reconcile this copy of the plan and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, plan)

	if equality.Semantic.DeepEqual(original.Status, plan.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(plan); uErr != nil {
		logger.Warn("Failed to update plan status", zap.Error(uErr))
		c.Recorder.Eventf(plan, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for Plan %q: %v", plan.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(plan, corev1.EventTypeNormal, "Updated", "Updated Plan %q", plan.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, plan *planv1alpha1.Plan) error {
	logger := logging.FromContext(ctx)
	if plan.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	plan.SetDefaults(context.Background())

	plan.Status.InitializeConditions()

	buildplanName := resourcenames.BuildPlan(plan)
	buildplan, err := c.buildplanLister.BuildPlans(plan.Namespace).Get(buildplanName)
	if errors.IsNotFound(err) {
		buildplan, err = c.createBuildPlan(plan)
		if err != nil {
			logger.Errorf("Failed to create BuildPlan %q: %v", buildplanName, err)
			c.Recorder.Eventf(plan, corev1.EventTypeWarning, "CreationFailed", "Failed to create BuildPlan %q: %v", buildplanName, err)
			return err
		}
		if buildplan == nil {
			c.Recorder.Eventf(plan, corev1.EventTypeNormal, "Skipped", "Skipped creating BuildPlan")
		} else {
			c.Recorder.Eventf(plan, corev1.EventTypeNormal, "Created", "Created BuildPlan %q", buildplanName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Plan: %q failed to Get BuildPlan: %q; %v", plan.Name, buildplanName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(buildplan, plan) {
		// Surface an error in the plan's status,and return an error.
		plan.Status.MarkBuildPlanNotOwned(buildplanName)
		return fmt.Errorf("Plan: %q does not own BuildPlan: %q", plan.Name, buildplanName)
	} else if buildplan, err = c.reconcileBuildPlan(ctx, plan, buildplan); err != nil {
		logger.Errorf("Failed to reconcile Plan: %q failed to reconcile BuildPlan: %q; %v", plan.Name, buildplanName, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying BuildPlan.
	plan.Status.PropagateBuildPlanStatus(buildplan)
	if buildplan == nil && plan.Status.LatestImage == "" {
		plan.Status.LatestImage = plan.Spec.Image
	}

	if plan.Status.LatestImage != "" {
		runplanName := resourcenames.RunPlan(plan)
		runplan, err := c.runplanLister.RunPlans(plan.Namespace).Get(runplanName)
		if errors.IsNotFound(err) {
			runplan, err = c.createRunPlan(plan)
			if err != nil {
				logger.Errorf("Failed to create RunPlan %q: %v", runplanName, err)
				c.Recorder.Eventf(plan, corev1.EventTypeWarning, "CreationFailed", "Failed to create RunPlan %q: %v", runplanName, err)
				return err
			}
			if runplan == nil {
				c.Recorder.Eventf(plan, corev1.EventTypeNormal, "Skipped", "Skipped creating RunPlan")
			} else {
				c.Recorder.Eventf(plan, corev1.EventTypeNormal, "Created", "Created RunPlan %q", runplanName)
			}
		} else if err != nil {
			logger.Errorf("Failed to reconcile Plan: %q failed to Get RunPlan: %q; %v", plan.Name, runplanName, zap.Error(err))
			return err
		} else if !metav1.IsControlledBy(runplan, plan) {
			// Surface an error in the plan's status,and return an error.
			plan.Status.MarkRunPlanNotOwned(runplanName)
			return fmt.Errorf("Plan: %q does not own RunPlan: %q", plan.Name, runplanName)
		} else if runplan, err = c.reconcileRunPlan(ctx, plan, runplan); err != nil {
			logger.Errorf("Failed to reconcile Plan: %q failed to reconcile RunPlan: %q; %v", plan.Name, runplanName, zap.Error(err))
			return err
		}

		// Update our Status based on the state of our underlying RunPlan.
		plan.Status.PropagateRunPlanStatus(runplan)
	}

	routeName := resourcenames.Route(plan)
	route, err := c.routeLister.Routes(plan.Namespace).Get(routeName)
	if errors.IsNotFound(err) {
		if plan.Status.ConfigurationName != "" {
			route, err = c.createRoute(plan)
			if err != nil {
				logger.Errorf("Failed to create Route %q: %v", routeName, err)
				c.Recorder.Eventf(plan, corev1.EventTypeWarning, "CreationFailed", "Failed to create Route %q: %v", routeName, err)
				return err
			}
			if route == nil {
				c.Recorder.Eventf(plan, corev1.EventTypeNormal, "Skipped", "Skipped creating Route")
			} else {
				c.Recorder.Eventf(plan, corev1.EventTypeNormal, "Created", "Created Route %q", routeName)
			}
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Plan: %q failed to Get Route: %q; %v", plan.Name, routeName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(route, plan) {
		// Surface an error in the plan's status,and return an error.
		plan.Status.MarkRouteNotOwned(routeName)
		return fmt.Errorf("Plan: %q does not own Route: %q", plan.Name, routeName)
	} else if route, err = c.reconcileRoute(ctx, plan, route); err != nil {
		logger.Errorf("Failed to reconcile Plan: %q failed to reconcile Route: %q; %v", plan.Name, routeName, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Route.
	plan.Status.PropagateRouteStatus(route)

	plan.Status.ObservedGeneration = plan.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *planv1alpha1.Plan) (*planv1alpha1.Plan, error) {
	plan, err := c.planLister.Plans(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(plan.Status, desired.Status) {
		return plan, nil
	}
	becomesReady := desired.Status.IsReady() && !plan.Status.IsReady()
	// Don't modify the informers copy.
	existing := plan.DeepCopy()
	existing.Status = desired.Status

	plan, err = c.ProjectriffClientSet.PlanV1alpha1().Plans(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(plan.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("Plan %q became ready after %v", plan.Name, duration)
	}

	return plan, err
}

func (c *Reconciler) reconcileBuildPlan(ctx context.Context, plan *planv1alpha1.Plan, buildplan *planv1alpha1.BuildPlan) (*planv1alpha1.BuildPlan, error) {
	logger := logging.FromContext(ctx)
	desiredBuildplan, err := resources.MakeBuildPlan(plan)
	if err != nil {
		return nil, err
	}

	if desiredBuildplan == nil {
		err = c.ProjectriffClientSet.PlanV1alpha1().BuildPlans(plan.Namespace).Delete(buildplan.Name, &metav1.DeleteOptions{
			Preconditions: &metav1.Preconditions{UID: &buildplan.UID},
		})
		if err != nil {
			return nil, err
		}
		c.Recorder.Eventf(plan, corev1.EventTypeNormal, "Deleted", "Deleted BuildPlan %q", buildplan.Name)
		return nil, nil
	}

	if buildplanSemanticEquals(desiredBuildplan, buildplan) {
		// No differences to reconcile.
		return buildplan, nil
	}
	diff, err := kmp.SafeDiff(desiredBuildplan.Spec, buildplan.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff BuildPlan: %v", err)
	}
	logger.Infof("Reconciling buildplan diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := buildplan.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredBuildplan.Spec
	existing.ObjectMeta.Labels = desiredBuildplan.ObjectMeta.Labels
	return c.ProjectriffClientSet.PlanV1alpha1().BuildPlans(plan.Namespace).Update(existing)
}

func (c *Reconciler) createBuildPlan(plan *planv1alpha1.Plan) (*planv1alpha1.BuildPlan, error) {
	buildplan, err := resources.MakeBuildPlan(plan)
	if err != nil {
		return nil, err
	}
	if buildplan == nil {
		// nothing to create
		return buildplan, nil
	}
	return c.ProjectriffClientSet.PlanV1alpha1().BuildPlans(plan.Namespace).Create(buildplan)
}

func buildplanSemanticEquals(desiredBuildplan, buildplan *planv1alpha1.BuildPlan) bool {
	return equality.Semantic.DeepEqual(desiredBuildplan.Spec, buildplan.Spec) &&
		equality.Semantic.DeepEqual(desiredBuildplan.ObjectMeta.Labels, buildplan.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileRunPlan(ctx context.Context, plan *planv1alpha1.Plan, runplan *planv1alpha1.RunPlan) (*planv1alpha1.RunPlan, error) {
	logger := logging.FromContext(ctx)
	desiredRunplan, err := resources.MakeRunPlan(plan)
	if err != nil {
		return nil, err
	}

	if desiredRunplan == nil {
		err = c.ProjectriffClientSet.PlanV1alpha1().RunPlans(plan.Namespace).Delete(runplan.Name, &metav1.DeleteOptions{
			Preconditions: &metav1.Preconditions{UID: &runplan.UID},
		})
		if err != nil {
			return nil, err
		}
		c.Recorder.Eventf(plan, corev1.EventTypeNormal, "Deleted", "Deleted RunPlan %q", runplan.Name)
		return nil, nil
	}

	if runplanSemanticEquals(desiredRunplan, runplan) {
		// No differences to reconcile.
		return runplan, nil
	}
	diff, err := kmp.SafeDiff(desiredRunplan.Spec, runplan.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff RunPlan: %v", err)
	}
	logger.Infof("Reconciling runplan diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := runplan.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredRunplan.Spec
	existing.ObjectMeta.Labels = desiredRunplan.ObjectMeta.Labels
	return c.ProjectriffClientSet.PlanV1alpha1().RunPlans(plan.Namespace).Update(existing)
}

func (c *Reconciler) createRunPlan(plan *planv1alpha1.Plan) (*planv1alpha1.RunPlan, error) {
	runplan, err := resources.MakeRunPlan(plan)
	if err != nil {
		return nil, err
	}
	if runplan == nil {
		// nothing to create
		return runplan, nil
	}
	return c.ProjectriffClientSet.PlanV1alpha1().RunPlans(plan.Namespace).Create(runplan)
}

func runplanSemanticEquals(desiredRunplan, runplan *planv1alpha1.RunPlan) bool {
	return equality.Semantic.DeepEqual(desiredRunplan.Spec, runplan.Spec) &&
		equality.Semantic.DeepEqual(desiredRunplan.ObjectMeta.Labels, runplan.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileRoute(ctx context.Context, plan *planv1alpha1.Plan, route *servingv1alpha1.Route) (*servingv1alpha1.Route, error) {
	logger := logging.FromContext(ctx)
	desiredRoute, err := resources.MakeRoute(plan)
	if err != nil {
		return nil, err
	}

	if desiredRoute == nil {
		err = c.ServingClientSet.ServingV1alpha1().Routes(route.Namespace).Delete(route.Name, &metav1.DeleteOptions{
			Preconditions: &metav1.Preconditions{UID: &route.UID},
		})
		if err != nil {
			return nil, err
		}
		c.Recorder.Eventf(plan, corev1.EventTypeNormal, "Deleted", "Deleted Route %q", route.Name)
		return nil, nil
	}

	if routeSemanticEquals(desiredRoute, route) {
		// No differences to reconcile.
		return route, nil
	}
	diff, err := kmp.SafeDiff(desiredRoute.Spec, route.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff Route: %v", err)
	}
	logger.Infof("Reconciling route diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := route.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredRoute.Spec
	existing.ObjectMeta.Labels = desiredRoute.ObjectMeta.Labels
	return c.ServingClientSet.ServingV1alpha1().Routes(plan.Namespace).Update(existing)
}

func (c *Reconciler) createRoute(plan *planv1alpha1.Plan) (*servingv1alpha1.Route, error) {
	route, err := resources.MakeRoute(plan)
	if err != nil {
		return nil, err
	}
	if route == nil {
		// nothing to create
		return route, nil
	}
	return c.ServingClientSet.ServingV1alpha1().Routes(plan.Namespace).Create(route)
}

func routeSemanticEquals(desiredRoute, route *servingv1alpha1.Route) bool {
	return equality.Semantic.DeepEqual(desiredRoute.Spec, route.Spec) &&
		equality.Semantic.DeepEqual(desiredRoute.ObjectMeta.Labels, route.ObjectMeta.Labels)
}
