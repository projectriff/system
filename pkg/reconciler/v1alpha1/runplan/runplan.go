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

package runplan

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
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/runplan/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/runplan/resources/names"
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
	ReconcilerName      = "RunPlans"
	controllerAgentName = "runplan-controller"
)

// Reconciler implements controller.Reconciler for RunPlan resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	runplanLister       planlisters.RunPlanLister
	configurationLister servinglisters.ConfigurationLister
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	runplanInformer planinformers.RunPlanInformer,
	configurationInformer servinginformers.ConfigurationInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:                reconciler.NewBase(opt, controllerAgentName),
		runplanLister:       runplanInformer.Lister(),
		configurationLister: configurationInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	runplanInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
		DeleteFunc: impl.Enqueue,
	})

	configurationInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(planv1alpha1.SchemeGroupVersion.WithKind("RunPlan")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the RunPlan resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the RunPlan resource with this namespace/name
	original, err := c.runplanLister.RunPlans(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("runplan %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	runplan := original.DeepCopy()

	// Reconcile this copy of the runplan and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, runplan)

	if equality.Semantic.DeepEqual(original.Status, runplan.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(runplan); uErr != nil {
		logger.Warn("Failed to update runplan status", zap.Error(uErr))
		c.Recorder.Eventf(runplan, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for RunPlan %q: %v", runplan.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(runplan, corev1.EventTypeNormal, "Updated", "Updated RunPlan %q", runplan.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, runplan *planv1alpha1.RunPlan) error {
	logger := logging.FromContext(ctx)
	if runplan.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	runplan.SetDefaults(context.Background())

	runplan.Status.InitializeConditions()

	configurationName := resourcenames.Configuration(runplan)
	configuration, err := c.configurationLister.Configurations(runplan.Namespace).Get(configurationName)
	if errors.IsNotFound(err) {
		configuration, err = c.createConfiguration(runplan)
		if err != nil {
			logger.Errorf("Failed to create Configuration %q: %v", configurationName, err)
			c.Recorder.Eventf(runplan, corev1.EventTypeWarning, "CreationFailed", "Failed to create Configuration %q: %v", configurationName, err)
			return err
		}
		c.Recorder.Eventf(runplan, corev1.EventTypeNormal, "Created", "Created Configuration %q", configurationName)
	} else if err != nil {
		logger.Errorf("Failed to reconcile RunPlan: %q failed to Get Configuration: %q; %v", runplan.Name, configurationName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(configuration, runplan) {
		// Surface an error in the runplan's status,and return an error.
		runplan.Status.MarkConfigurationNotOwned(configurationName)
		return fmt.Errorf("RunPlan: %q does not own Configuration: %q", runplan.Name, configurationName)
	} else if configuration, err = c.reconcileConfiguration(ctx, runplan, configuration); err != nil {
		logger.Errorf("Failed to reconcile RunPlan: %q failed to reconcile Configuration: %q; %v", runplan.Name, configurationName, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Configuration.
	runplan.Status.PropagateConfigurationStatus(configuration)

	runplan.Status.ObservedGeneration = runplan.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *planv1alpha1.RunPlan) (*planv1alpha1.RunPlan, error) {
	runplan, err := c.runplanLister.RunPlans(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(runplan.Status, desired.Status) {
		return runplan, nil
	}
	becomesReady := desired.Status.IsReady() && !runplan.Status.IsReady()
	// Don't modify the informers copy.
	existing := runplan.DeepCopy()
	existing.Status = desired.Status

	svc, err := c.ProjectriffClientSet.PlanV1alpha1().RunPlans(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(svc.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("RunPlan %q became ready after %v", runplan.Name, duration)
	}

	return svc, err
}

func (c *Reconciler) reconcileConfiguration(ctx context.Context, runplan *planv1alpha1.RunPlan, configuration *servingv1alpha1.Configuration) (*servingv1alpha1.Configuration, error) {
	logger := logging.FromContext(ctx)
	desiredConfiguration, err := resources.MakeConfiguration(runplan)
	if err != nil {
		return nil, err
	}

	if configurationSemanticEquals(desiredConfiguration, configuration) {
		// No differences to reconcile.
		return configuration, nil
	}
	diff, err := kmp.SafeDiff(desiredConfiguration.Spec, configuration.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff Configuration: %v", err)
	}
	logger.Infof("Reconciling configuration diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := configuration.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredConfiguration.Spec
	existing.ObjectMeta.Labels = desiredConfiguration.ObjectMeta.Labels
	return c.ServingClientSet.ServingV1alpha1().Configurations(runplan.Namespace).Update(existing)
}

func (c *Reconciler) createConfiguration(runplan *planv1alpha1.RunPlan) (*servingv1alpha1.Configuration, error) {
	configuration, err := resources.MakeConfiguration(runplan)
	if err != nil {
		return nil, err
	}
	return c.ServingClientSet.ServingV1alpha1().Configurations(runplan.Namespace).Create(configuration)
}

func configurationSemanticEquals(desiredConfiguration, configuration *servingv1alpha1.Configuration) bool {
	return equality.Semantic.DeepEqual(desiredConfiguration.Spec, configuration.Spec) &&
		equality.Semantic.DeepEqual(desiredConfiguration.ObjectMeta.Labels, configuration.ObjectMeta.Labels)
}
