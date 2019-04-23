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

package applicationbuild

import (
	"context"
	"fmt"
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
	"github.com/projectriff/system/pkg/reconciler/digest"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/applicationbuild/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/applicationbuild/resources/names"
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

const (
	// ReconcilerName is the name of the reconciler
	ReconcilerName      = "ApplicationBuilds"
	controllerAgentName = "applicationbuild-controller"
)

// Reconciler implements controller.Reconciler for ApplicationBuild resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	applicationbuildLister buildlisters.ApplicationBuildLister
	pvcLister              corelisters.PersistentVolumeClaimLister
	knbuildLister          knbuildlisters.BuildLister

	resolver digest.Resolver
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	applicationbuildInformer buildinformers.ApplicationBuildInformer,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
	knbuildInformer knbuildinformers.BuildInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:                   reconciler.NewBase(opt, controllerAgentName),
		applicationbuildLister: applicationbuildInformer.Lister(),
		pvcLister:              pvcInformer.Lister(),
		knbuildLister:          knbuildInformer.Lister(),

		resolver: digest.NewDefaultResolver(opt),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	applicationbuildInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
		DeleteFunc: impl.Enqueue,
	})

	pvcInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(buildv1alpha1.SchemeGroupVersion.WithKind("ApplicationBuild")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})
	knbuildInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(buildv1alpha1.SchemeGroupVersion.WithKind("ApplicationBuild")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the ApplicationBuild resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the ApplicationBuild resource with this namespace/name
	original, err := c.applicationbuildLister.ApplicationBuilds(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("applicationbuild %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	applicationbuild := original.DeepCopy()

	// Reconcile this copy of the applicationbuild and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, applicationbuild)

	if equality.Semantic.DeepEqual(original.Status, applicationbuild.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(applicationbuild); uErr != nil {
		logger.Warn("Failed to update applicationbuild status", zap.Error(uErr))
		c.Recorder.Eventf(applicationbuild, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for ApplicationBuild %q: %v", applicationbuild.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(applicationbuild, corev1.EventTypeNormal, "Updated", "Updated ApplicationBuild %q", applicationbuild.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, applicationbuild *buildv1alpha1.ApplicationBuild) error {
	logger := logging.FromContext(ctx)
	if applicationbuild.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	applicationbuild.SetDefaults(context.Background())

	applicationbuild.Status.InitializeConditions()

	buildCacheName := resourcenames.BuildCache(applicationbuild)
	buildCache, err := c.pvcLister.PersistentVolumeClaims(applicationbuild.Namespace).Get(buildCacheName)
	if errors.IsNotFound(err) {
		buildCache, err = c.createBuildCache(applicationbuild)
		if err != nil {
			logger.Errorf("Failed to create PersistentVolumeClaim %q: %v", buildCacheName, err)
			c.Recorder.Eventf(applicationbuild, corev1.EventTypeWarning, "CreationFailed", "Failed to create PersistentVolumeClaim %q: %v", buildCacheName, err)
			return err
		}
		if buildCache != nil {
			c.Recorder.Eventf(applicationbuild, corev1.EventTypeNormal, "Created", "Created PersistentVolumeClaim %q", buildCacheName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile ApplicationBuild: %q failed to Get PersistentVolumeClaim: %q; %v", applicationbuild.Name, buildCacheName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(buildCache, applicationbuild) {
		// Surface an error in the applicationbuild's status,and return an error.
		applicationbuild.Status.MarkBuildCacheNotOwned(buildCacheName)
		return fmt.Errorf("ApplicationBuild: %q does not own PersistentVolumeClaim: %q", applicationbuild.Name, buildCacheName)
	} else {
		buildCache, err = c.reconcileBuildCache(ctx, applicationbuild, buildCache)
		if err != nil {
			logger.Errorf("Failed to reconcile ApplicationBuild: %q failed to reconcile PersistentVolumeClaim: %q; %v", applicationbuild.Name, buildCache, zap.Error(err))
			return err
		}
		if buildCache == nil {
			c.Recorder.Eventf(applicationbuild, corev1.EventTypeNormal, "Deleted", "Deleted PersistentVolumeClaim %q", buildCacheName)
		}
	}

	// Update our Status based on the state of our underlying PersistentVolumeClaim.
	if buildCache == nil {
		applicationbuild.Status.MarkBuildCacheNotUsed()
	} else {
		applicationbuild.Status.BuildCacheName = buildCache.Name
		applicationbuild.Status.PropagateBuildCacheStatus(&buildCache.Status)
	}

	buildName := resourcenames.Build(applicationbuild)
	build, err := c.knbuildLister.Builds(applicationbuild.Namespace).Get(buildName)
	if errors.IsNotFound(err) {
		build, err = c.createBuild(applicationbuild)
		if err != nil {
			logger.Errorf("Failed to create Build %q: %v", buildName, err)
			c.Recorder.Eventf(applicationbuild, corev1.EventTypeWarning, "CreationFailed", "Failed to create Build %q: %v", buildName, err)
			return err
		}
		if build != nil {
			c.Recorder.Eventf(applicationbuild, corev1.EventTypeNormal, "Created", "Created Build %q", buildName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile ApplicationBuild: %q failed to Get Build: %q; %v", applicationbuild.Name, buildName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(build, applicationbuild) {
		// Surface an error in the applicationbuild's status,and return an error.
		applicationbuild.Status.MarkBuildNotOwned(buildName)
		return fmt.Errorf("ApplicationBuild: %q does not own Build: %q", applicationbuild.Name, buildName)
	} else if build, err = c.reconcileBuild(ctx, applicationbuild, build); err != nil {
		logger.Errorf("Failed to reconcile ApplicationBuild: %q failed to reconcile Build: %q; %v", applicationbuild.Name, build, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Build.
	applicationbuild.Status.BuildName = build.Name
	applicationbuild.Status.PropagateBuildStatus(&build.Status)
	if applicationbuild.Status.IsReady() {
		// resolve image name
		opt := k8schain.Options{
			Namespace:          applicationbuild.Namespace,
			ServiceAccountName: build.Spec.ServiceAccountName,
		}
		// TODO load from a configmap
		skipRegistries := sets.NewString()
		skipRegistries.Insert("ko.local")
		skipRegistries.Insert("dev.local")
		digest, err := c.resolver.Resolve(applicationbuild.Spec.Image, opt, skipRegistries)
		if err != nil {
			applicationbuild.Status.MarkImageMissing(fmt.Sprintf("Unable to fetch image %q: %s", applicationbuild.Spec.Image, err.Error()))
			return err
		}
		applicationbuild.Status.LatestImage = digest
	}

	applicationbuild.Status.ObservedGeneration = applicationbuild.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *buildv1alpha1.ApplicationBuild) (*buildv1alpha1.ApplicationBuild, error) {
	applicationbuild, err := c.applicationbuildLister.ApplicationBuilds(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(applicationbuild.Status, desired.Status) {
		return applicationbuild, nil
	}
	becomesReady := desired.Status.IsReady() && !applicationbuild.Status.IsReady()
	// Don't modify the informers copy.
	existing := applicationbuild.DeepCopy()
	existing.Status = desired.Status

	fb, err := c.ProjectriffClientSet.BuildV1alpha1().ApplicationBuilds(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(fb.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("ApplicationBuild %q became ready after %v", applicationbuild.Name, duration)
	}

	return fb, err
}

func (c *Reconciler) reconcileBuildCache(ctx context.Context, applicationbuild *buildv1alpha1.ApplicationBuild, buildCache *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	logger := logging.FromContext(ctx)
	desiredBuildCache, err := resources.MakeBuildCache(applicationbuild)
	if err != nil {
		return nil, err
	}

	if desiredBuildCache == nil {
		// drop an existing buildCache
		return nil, c.KubeClientSet.CoreV1().PersistentVolumeClaims(applicationbuild.Namespace).Delete(buildCache.Name, &metav1.DeleteOptions{
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
	return c.KubeClientSet.CoreV1().PersistentVolumeClaims(applicationbuild.Namespace).Update(existing)
}

func (c *Reconciler) createBuildCache(applicationbuild *buildv1alpha1.ApplicationBuild) (*corev1.PersistentVolumeClaim, error) {
	buildCache, err := resources.MakeBuildCache(applicationbuild)
	if err != nil {
		return nil, err
	}
	if buildCache == nil {
		// nothing to create
		return buildCache, nil
	}
	return c.KubeClientSet.CoreV1().PersistentVolumeClaims(applicationbuild.Namespace).Create(buildCache)
}

func buildCacheSemanticEquals(desiredBuildCache, buildCache *corev1.PersistentVolumeClaim) bool {
	return equality.Semantic.DeepEqual(desiredBuildCache.Spec.Resources, buildCache.Spec.Resources) &&
		equality.Semantic.DeepEqual(desiredBuildCache.ObjectMeta.Labels, buildCache.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileBuild(ctx context.Context, applicationbuild *buildv1alpha1.ApplicationBuild, build *knbuildv1alpha1.Build) (*knbuildv1alpha1.Build, error) {
	logger := logging.FromContext(ctx)
	desiredBuild, err := resources.MakeBuild(applicationbuild)
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
	return c.KnBuildClientSet.BuildV1alpha1().Builds(applicationbuild.Namespace).Update(existing)
}

func (c *Reconciler) createBuild(applicationbuild *buildv1alpha1.ApplicationBuild) (*knbuildv1alpha1.Build, error) {
	build, err := resources.MakeBuild(applicationbuild)
	if err != nil {
		return nil, err
	}
	if build == nil {
		// nothing to create
		return build, nil
	}
	return c.KnBuildClientSet.BuildV1alpha1().Builds(applicationbuild.Namespace).Create(build)
}

func buildSemanticEquals(desiredBuild, build *knbuildv1alpha1.Build) bool {
	return equality.Semantic.DeepEqual(desiredBuild.Spec, build.Spec) &&
		equality.Semantic.DeepEqual(desiredBuild.ObjectMeta.Labels, build.ObjectMeta.Labels)
}
