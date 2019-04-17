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

package buildplan

import (
	"context"
	"fmt"
	"reflect"
	"time"

	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	buildinformers "github.com/knative/build/pkg/client/informers/externalversions/build/v1alpha1"
	buildlisters "github.com/knative/build/pkg/client/listers/build/v1alpha1"
	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/kmp"
	"github.com/knative/pkg/logging"
	planv1alpha1 "github.com/projectriff/system/pkg/apis/plan/v1alpha1"
	planinformers "github.com/projectriff/system/pkg/client/informers/externalversions/plan/v1alpha1"
	planlisters "github.com/projectriff/system/pkg/client/listers/plan/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/buildplan/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/buildplan/resources/names"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	// ReconcilerName is the name of the reconciler
	ReconcilerName      = "BuildPlans"
	controllerAgentName = "buildplan-controller"
)

// Reconciler implements controller.Reconciler for BuildPlan resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	buildplanLister planlisters.BuildPlanLister
	pvcLister       corelisters.PersistentVolumeClaimLister
	buildLister     buildlisters.BuildLister
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	buildplanInformer planinformers.BuildPlanInformer,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
	buildInformer buildinformers.BuildInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:            reconciler.NewBase(opt, controllerAgentName),
		buildplanLister: buildplanInformer.Lister(),
		pvcLister:       pvcInformer.Lister(),
		buildLister:     buildInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	buildplanInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
		DeleteFunc: impl.Enqueue,
	})

	pvcInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(planv1alpha1.SchemeGroupVersion.WithKind("BuildPlan")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})
	buildInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(planv1alpha1.SchemeGroupVersion.WithKind("BuildPlan")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the BuildPlan resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the BuildPlan resource with this namespace/name
	original, err := c.buildplanLister.BuildPlans(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("buildplan %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	buildplan := original.DeepCopy()

	// Reconcile this copy of the buildplan and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, buildplan)

	if equality.Semantic.DeepEqual(original.Status, buildplan.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(buildplan); uErr != nil {
		logger.Warn("Failed to update buildplan status", zap.Error(uErr))
		c.Recorder.Eventf(buildplan, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for BuildPlan %q: %v", buildplan.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(buildplan, corev1.EventTypeNormal, "Updated", "Updated BuildPlan %q", buildplan.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, buildplan *planv1alpha1.BuildPlan) error {
	logger := logging.FromContext(ctx)
	if buildplan.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	buildplan.SetDefaults(context.Background())

	buildplan.Status.InitializeConditions()

	buildCacheName := resourcenames.BuildCache(buildplan)
	buildCache, err := c.pvcLister.PersistentVolumeClaims(buildplan.Namespace).Get(buildCacheName)
	if errors.IsNotFound(err) {
		buildCache, err = c.createBuildCache(buildplan)
		if err != nil {
			logger.Errorf("Failed to create PersistentVolumeClaim %q: %v", buildCacheName, err)
			c.Recorder.Eventf(buildplan, corev1.EventTypeWarning, "CreationFailed", "Failed to create PersistentVolumeClaim %q: %v", buildCacheName, err)
			return err
		}
		if buildCache != nil {
			c.Recorder.Eventf(buildplan, corev1.EventTypeNormal, "Created", "Created PersistentVolumeClaim %q", buildCacheName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile BuildPlan: %q failed to Get PersistentVolumeClaim: %q; %v", buildplan.Name, buildCacheName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(buildCache, buildplan) {
		// Surface an error in the buildplan's status,and return an error.
		buildplan.Status.MarkBuildCacheNotOwned(buildCacheName)
		return fmt.Errorf("BuildPlan: %q does not own PersistentVolumeClaim: %q", buildplan.Name, buildCacheName)
	} else if buildCache, err = c.reconcileBuildCache(ctx, buildplan, buildCache); err != nil {
		logger.Errorf("Failed to reconcile BuildPlan: %q failed to reconcile PersistentVolumeClaim: %q; %v", buildplan.Name, buildCache, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying PersistentVolumeClaim.
	buildplan.Status.PropagateBuildCacheStatus(buildCache)

	buildName := resourcenames.Build(buildplan)
	build, err := c.buildLister.Builds(buildplan.Namespace).Get(buildName)
	if errors.IsNotFound(err) {
		build, err = c.createBuild(buildplan)
		if err != nil {
			logger.Errorf("Failed to create Build %q: %v", buildName, err)
			c.Recorder.Eventf(buildplan, corev1.EventTypeWarning, "CreationFailed", "Failed to create Build %q: %v", buildName, err)
			return err
		}
		if build != nil {
			c.Recorder.Eventf(buildplan, corev1.EventTypeNormal, "Created", "Created Build %q", buildName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile BuildPlan: %q failed to Get Build: %q; %v", buildplan.Name, buildName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(build, buildplan) {
		// Surface an error in the buildplan's status,and return an error.
		buildplan.Status.MarkBuildNotOwned(buildName)
		return fmt.Errorf("BuildPlan: %q does not own Build: %q", buildplan.Name, buildName)
	} else if build, err = c.reconcileBuild(ctx, buildplan, build); err != nil {
		logger.Errorf("Failed to reconcile BuildPlan: %q failed to reconcile Build: %q; %v", buildplan.Name, build, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Build.
	buildplan.Status.PropagateBuildStatus(build)
	if buildplan.Status.GetCondition(planv1alpha1.BuildPlanConditionBuildSucceeded).IsTrue() {
		// TODO resolve image to digest
		buildplan.Status.LatestBuiltImage = buildplan.Spec.Image
	}

	buildplan.Status.ObservedGeneration = buildplan.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *planv1alpha1.BuildPlan) (*planv1alpha1.BuildPlan, error) {
	buildplan, err := c.buildplanLister.BuildPlans(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(buildplan.Status, desired.Status) {
		return buildplan, nil
	}
	becomesReady := desired.Status.IsReady() && !buildplan.Status.IsReady()
	// Don't modify the informers copy.
	existing := buildplan.DeepCopy()
	existing.Status = desired.Status

	svc, err := c.ProjectriffClientSet.PlanV1alpha1().BuildPlans(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(svc.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("BuildPlan %q became ready after %v", buildplan.Name, duration)
	}

	return svc, err
}

func (c *Reconciler) reconcileBuildCache(ctx context.Context, buildplan *planv1alpha1.BuildPlan, buildCache *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	logger := logging.FromContext(ctx)
	desiredBuildCache, err := resources.MakeBuildCache(buildplan)
	if err != nil {
		return nil, err
	}

	if buildCache != nil && desiredBuildCache == nil {
		// TODO drop an existing buildCache
		// return nil, c.KubeClientSet.CoreV1().PersistentVolumeClaims(buildplan.Namespace).Delete(buildCache.Name)
		return nil, nil
	}

	if buildCacheSemanticEquals(desiredBuildCache, buildCache) {
		// No differences to reconcile.
		return buildCache, nil
	}
	diff, err := kmp.SafeDiff(desiredBuildCache.Spec, buildCache.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff PersistentVolumeClaim: %v", err)
	}
	logger.Infof("Reconciling build cache diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := buildCache.DeepCopy()
	// Preserve the rest of the object (e.g. PersistentVolumeClaimSpec except for resources).
	existing.Spec.Resources = desiredBuildCache.Spec.Resources
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.ObjectMeta.Labels = desiredBuildCache.ObjectMeta.Labels
	return c.KubeClientSet.CoreV1().PersistentVolumeClaims(buildplan.Namespace).Update(existing)
}

func (c *Reconciler) createBuildCache(buildplan *planv1alpha1.BuildPlan) (*corev1.PersistentVolumeClaim, error) {
	buildCache, err := resources.MakeBuildCache(buildplan)
	if err != nil {
		return nil, err
	}
	if buildCache == nil {
		// nothing to create
		return buildCache, nil
	}
	return c.KubeClientSet.CoreV1().PersistentVolumeClaims(buildplan.Namespace).Create(buildCache)
}

func buildCacheSemanticEquals(desiredBuildCache, buildCache *corev1.PersistentVolumeClaim) bool {
	return equality.Semantic.DeepEqual(desiredBuildCache.Spec.Resources, buildCache.Spec.Resources) &&
		equality.Semantic.DeepEqual(desiredBuildCache.ObjectMeta.Labels, buildCache.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileBuild(ctx context.Context, buildplan *planv1alpha1.BuildPlan, build *buildv1alpha1.Build) (*buildv1alpha1.Build, error) {
	logger := logging.FromContext(ctx)
	desiredBuild, err := resources.MakeBuild(buildplan)
	if err != nil {
		return nil, err
	}

	if build != nil && desiredBuild == nil {
		// TODO drop a existing build
		// return nil, c.buildClientSet.BuildV1alpha1().Builds(buildplan.Namespace).Delete(build.Name)
		return nil, nil
	}

	if buildSemanticEquals(desiredBuild, build) {
		// No differences to reconcile.
		return build, nil
	}
	diff, err := kmp.SafeDiff(desiredBuild.Spec, build.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff Build: %v", err)
	}
	logger.Infof("Reconciling build diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := build.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredBuild.Spec
	existing.ObjectMeta.Labels = desiredBuild.ObjectMeta.Labels
	return c.BuildClientSet.BuildV1alpha1().Builds(buildplan.Namespace).Update(existing)
}

func (c *Reconciler) createBuild(buildplan *planv1alpha1.BuildPlan) (*buildv1alpha1.Build, error) {
	build, err := resources.MakeBuild(buildplan)
	if err != nil {
		return nil, err
	}
	if build == nil {
		// nothing to create
		return build, nil
	}
	return c.BuildClientSet.BuildV1alpha1().Builds(buildplan.Namespace).Create(build)
}

func buildSemanticEquals(desiredBuild, build *buildv1alpha1.Build) bool {
	return equality.Semantic.DeepEqual(desiredBuild.Spec, build.Spec) &&
		equality.Semantic.DeepEqual(desiredBuild.ObjectMeta.Labels, build.ObjectMeta.Labels)
}
