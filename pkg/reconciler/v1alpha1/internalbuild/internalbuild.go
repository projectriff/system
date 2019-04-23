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

package internalbuild

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/google/go-containerregistry/pkg/authn/k8schain"
	knbuildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	knbuildinformers "github.com/knative/build/pkg/client/informers/externalversions/build/v1alpha1"
	knbuildlisters "github.com/knative/build/pkg/client/listers/build/v1alpha1"
	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/kmp"
	"github.com/knative/pkg/logging"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	buildinformers "github.com/projectriff/system/pkg/client/informers/externalversions/build/v1alpha1"
	buildlisters "github.com/projectriff/system/pkg/client/listers/build/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/internalbuild/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/internalbuild/resources/names"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	coreinformers "k8s.io/client-go/informers/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type resolver interface {
	Resolve(string, k8schain.Options, sets.String) (string, error)
}

const (
	// ReconcilerName is the name of the reconciler
	ReconcilerName      = "InternalBuilds"
	controllerAgentName = "internalbuild-controller"
)

// Reconciler implements controller.Reconciler for InternalBuild resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	internalbuildLister buildlisters.InternalBuildLister
	pvcLister           corelisters.PersistentVolumeClaimLister
	knbuildLister       knbuildlisters.BuildLister

	resolver *digestResolver
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	internalbuildInformer buildinformers.InternalBuildInformer,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
	knbuildInformer knbuildinformers.BuildInformer,
) *controller.Impl {
	transport := http.DefaultTransport
	if rt, err := newResolverTransport(k8sCertPath); err != nil {
		opt.Logger.Errorf("Failed to create resolver transport: %v", err)
	} else {
		transport = rt
	}

	c := &Reconciler{
		Base:                reconciler.NewBase(opt, controllerAgentName),
		internalbuildLister: internalbuildInformer.Lister(),
		pvcLister:           pvcInformer.Lister(),
		knbuildLister:       knbuildInformer.Lister(),

		resolver: &digestResolver{
			client:    opt.KubeClientSet,
			transport: transport,
		},
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	internalbuildInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
		DeleteFunc: impl.Enqueue,
	})

	pvcInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(buildv1alpha1.SchemeGroupVersion.WithKind("InternalBuild")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})
	knbuildInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(buildv1alpha1.SchemeGroupVersion.WithKind("InternalBuild")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the InternalBuild resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the InternalBuild resource with this namespace/name
	original, err := c.internalbuildLister.InternalBuilds(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("internalbuild %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	internalbuild := original.DeepCopy()

	// Reconcile this copy of the internalbuild and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, internalbuild)

	if equality.Semantic.DeepEqual(original.Status, internalbuild.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(internalbuild); uErr != nil {
		logger.Warn("Failed to update internalbuild status", zap.Error(uErr))
		c.Recorder.Eventf(internalbuild, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for InternalBuild %q: %v", internalbuild.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(internalbuild, corev1.EventTypeNormal, "Updated", "Updated InternalBuild %q", internalbuild.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, internalbuild *buildv1alpha1.InternalBuild) error {
	logger := logging.FromContext(ctx)
	if internalbuild.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	internalbuild.SetDefaults(context.Background())

	internalbuild.Status.InitializeConditions()

	buildCacheName := resourcenames.BuildCache(internalbuild)
	buildCache, err := c.pvcLister.PersistentVolumeClaims(internalbuild.Namespace).Get(buildCacheName)
	if errors.IsNotFound(err) {
		buildCache, err = c.createBuildCache(internalbuild)
		if err != nil {
			logger.Errorf("Failed to create PersistentVolumeClaim %q: %v", buildCacheName, err)
			c.Recorder.Eventf(internalbuild, corev1.EventTypeWarning, "CreationFailed", "Failed to create PersistentVolumeClaim %q: %v", buildCacheName, err)
			return err
		}
		if buildCache != nil {
			c.Recorder.Eventf(internalbuild, corev1.EventTypeNormal, "Created", "Created PersistentVolumeClaim %q", buildCacheName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile InternalBuild: %q failed to Get PersistentVolumeClaim: %q; %v", internalbuild.Name, buildCacheName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(buildCache, internalbuild) {
		// Surface an error in the internalbuild's status,and return an error.
		internalbuild.Status.MarkBuildCacheNotOwned(buildCacheName)
		return fmt.Errorf("InternalBuild: %q does not own PersistentVolumeClaim: %q", internalbuild.Name, buildCacheName)
	} else {
		buildCache, err = c.reconcileBuildCache(ctx, internalbuild, buildCache)
		if err != nil {
			logger.Errorf("Failed to reconcile InternalBuild: %q failed to reconcile PersistentVolumeClaim: %q; %v", internalbuild.Name, buildCache, zap.Error(err))
			return err
		}
		if buildCache == nil {
			c.Recorder.Eventf(internalbuild, corev1.EventTypeNormal, "Deleted", "Deleted PersistentVolumeClaim %q", buildCacheName)
		}
	}

	// Update our Status based on the state of our underlying PersistentVolumeClaim.
	if buildCache == nil {
		internalbuild.Status.MarkBuildCacheNotUsed()
	} else {
		internalbuild.Status.BuildCacheName = buildCache.Name
		internalbuild.Status.PropagateBuildCacheStatus(&buildCache.Status)
	}

	buildName := resourcenames.Build(internalbuild)
	build, err := c.knbuildLister.Builds(internalbuild.Namespace).Get(buildName)
	if errors.IsNotFound(err) {
		build, err = c.createBuild(internalbuild)
		if err != nil {
			logger.Errorf("Failed to create Build %q: %v", buildName, err)
			c.Recorder.Eventf(internalbuild, corev1.EventTypeWarning, "CreationFailed", "Failed to create Build %q: %v", buildName, err)
			return err
		}
		if build != nil {
			c.Recorder.Eventf(internalbuild, corev1.EventTypeNormal, "Created", "Created Build %q", buildName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile InternalBuild: %q failed to Get Build: %q; %v", internalbuild.Name, buildName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(build, internalbuild) {
		// Surface an error in the internalbuild's status,and return an error.
		internalbuild.Status.MarkBuildNotOwned(buildName)
		return fmt.Errorf("InternalBuild: %q does not own Build: %q", internalbuild.Name, buildName)
	} else if build, err = c.reconcileBuild(ctx, internalbuild, build); err != nil {
		logger.Errorf("Failed to reconcile InternalBuild: %q failed to reconcile Build: %q; %v", internalbuild.Name, build, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Build.
	internalbuild.Status.BuildName = build.Name
	internalbuild.Status.PropagateBuildStatus(&build.Status)
	if internalbuild.Status.IsReady() {
		// resolve image name
		opt := k8schain.Options{
			Namespace:          internalbuild.Namespace,
			ServiceAccountName: build.Spec.ServiceAccountName,
		}
		// TODO load from a configmap
		skipRegistries := sets.NewString()
		skipRegistries.Insert("ko.local")
		skipRegistries.Insert("dev.local")
		digest, err := c.resolver.Resolve(internalbuild.Spec.Image, opt, skipRegistries)
		if err != nil {
			internalbuild.Status.MarkImageMissing(fmt.Sprintf("Unable to fetch image %q: %s", internalbuild.Spec.Image, err.Error()))
			return err
		}
		internalbuild.Status.LatestImage = digest
	}

	internalbuild.Status.ObservedGeneration = internalbuild.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *buildv1alpha1.InternalBuild) (*buildv1alpha1.InternalBuild, error) {
	internalbuild, err := c.internalbuildLister.InternalBuilds(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(internalbuild.Status, desired.Status) {
		return internalbuild, nil
	}
	becomesReady := desired.Status.IsReady() && !internalbuild.Status.IsReady()
	// Don't modify the informers copy.
	existing := internalbuild.DeepCopy()
	existing.Status = desired.Status

	ib, err := c.ProjectriffClientSet.BuildV1alpha1().InternalBuilds(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(ib.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("InternalBuild %q became ready after %v", internalbuild.Name, duration)
	}

	return ib, err
}

func (c *Reconciler) reconcileBuildCache(ctx context.Context, internalbuild *buildv1alpha1.InternalBuild, buildCache *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	logger := logging.FromContext(ctx)
	desiredBuildCache, err := resources.MakeBuildCache(internalbuild)
	if err != nil {
		return nil, err
	}

	if desiredBuildCache == nil {
		// drop an existing buildCache
		return nil, c.KubeClientSet.CoreV1().PersistentVolumeClaims(internalbuild.Namespace).Delete(buildCache.Name, &metav1.DeleteOptions{
			Preconditions: &metav1.Preconditions{UID: &buildCache.UID},
		})
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
	return c.KubeClientSet.CoreV1().PersistentVolumeClaims(internalbuild.Namespace).Update(existing)
}

func (c *Reconciler) createBuildCache(internalbuild *buildv1alpha1.InternalBuild) (*corev1.PersistentVolumeClaim, error) {
	buildCache, err := resources.MakeBuildCache(internalbuild)
	if err != nil {
		return nil, err
	}
	if buildCache == nil {
		// nothing to create
		return buildCache, nil
	}
	return c.KubeClientSet.CoreV1().PersistentVolumeClaims(internalbuild.Namespace).Create(buildCache)
}

func buildCacheSemanticEquals(desiredBuildCache, buildCache *corev1.PersistentVolumeClaim) bool {
	return equality.Semantic.DeepEqual(desiredBuildCache.Spec.Resources, buildCache.Spec.Resources) &&
		equality.Semantic.DeepEqual(desiredBuildCache.ObjectMeta.Labels, buildCache.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileBuild(ctx context.Context, internalbuild *buildv1alpha1.InternalBuild, build *knbuildv1alpha1.Build) (*knbuildv1alpha1.Build, error) {
	logger := logging.FromContext(ctx)
	desiredBuild, err := resources.MakeBuild(internalbuild)
	if err != nil {
		return nil, err
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
	return c.KnBuildClientSet.BuildV1alpha1().Builds(internalbuild.Namespace).Update(existing)
}

func (c *Reconciler) createBuild(internalbuild *buildv1alpha1.InternalBuild) (*knbuildv1alpha1.Build, error) {
	build, err := resources.MakeBuild(internalbuild)
	if err != nil {
		return nil, err
	}
	if build == nil {
		// nothing to create
		return build, nil
	}
	return c.KnBuildClientSet.BuildV1alpha1().Builds(internalbuild.Namespace).Create(build)
}

func buildSemanticEquals(desiredBuild, build *knbuildv1alpha1.Build) bool {
	return equality.Semantic.DeepEqual(desiredBuild.Spec, build.Spec) &&
		equality.Semantic.DeepEqual(desiredBuild.ObjectMeta.Labels, build.ObjectMeta.Labels)
}
