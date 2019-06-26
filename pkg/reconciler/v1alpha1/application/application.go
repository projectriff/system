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

package application

import (
	"context"
	"fmt"
	"reflect"
	"strings"
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
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/application/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/application/resources/names"
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
	ReconcilerName      = "Applications"
	controllerAgentName = "application-controller"
)

var errMissingDefaultPrefix = fmt.Errorf("missing default image prefix")

// Reconciler implements controller.Reconciler for Application resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	applicationLister buildlisters.ApplicationLister
	configmapLister   corelisters.ConfigMapLister
	pvcLister         corelisters.PersistentVolumeClaimLister
	knbuildLister     knbuildlisters.BuildLister

	resolver digest.Resolver
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	applicationInformer buildinformers.ApplicationInformer,
	configmapInformer coreinformers.ConfigMapInformer,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
	knbuildInformer knbuildinformers.BuildInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:              reconciler.NewBase(opt, controllerAgentName),
		applicationLister: applicationInformer.Lister(),
		configmapLister:   configmapInformer.Lister(),
		pvcLister:         pvcInformer.Lister(),
		knbuildLister:     knbuildInformer.Lister(),

		resolver: digest.NewDefaultResolver(opt),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName)

	c.Logger.Info("Setting up event handlers")
	applicationInformer.Informer().AddEventHandler(reconciler.Handler(impl.Enqueue))

	// controlled resources
	pvcInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(buildv1alpha1.SchemeGroupVersion.WithKind("Application")),
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})
	knbuildInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(buildv1alpha1.SchemeGroupVersion.WithKind("Application")),
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Application resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the Application resource with this namespace/name
	original, err := c.applicationLister.Applications(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("application %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	application := original.DeepCopy()

	// Reconcile this copy of the application and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, application)

	if equality.Semantic.DeepEqual(original.Status, application.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(application); uErr != nil {
		logger.Warn("Failed to update application status", zap.Error(uErr))
		c.Recorder.Eventf(application, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for Application %q: %v", application.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(application, corev1.EventTypeNormal, "Updated", "Updated Application %q", application.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, application *buildv1alpha1.Application) error {
	logger := logging.FromContext(ctx)
	if application.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	application.SetDefaults(context.Background())

	application.Status.InitializeConditions()

	buildCacheName := resourcenames.BuildCache(application)
	buildCache, err := c.pvcLister.PersistentVolumeClaims(application.Namespace).Get(buildCacheName)
	if errors.IsNotFound(err) {
		buildCache, err = c.createBuildCache(application)
		if err != nil {
			logger.Errorf("Failed to create PersistentVolumeClaim %q: %v", buildCacheName, err)
			c.Recorder.Eventf(application, corev1.EventTypeWarning, "CreationFailed", "Failed to create PersistentVolumeClaim %q: %v", buildCacheName, err)
			return err
		}
		if buildCache != nil {
			c.Recorder.Eventf(application, corev1.EventTypeNormal, "Created", "Created PersistentVolumeClaim %q", buildCacheName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Application: %q failed to Get PersistentVolumeClaim: %q; %v", application.Name, buildCacheName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(buildCache, application) {
		// Surface an error in the application's status,and return an error.
		application.Status.MarkBuildCacheNotOwned(buildCacheName)
		return fmt.Errorf("Application: %q does not own PersistentVolumeClaim: %q", application.Name, buildCacheName)
	} else {
		buildCache, err = c.reconcileBuildCache(ctx, application, buildCache)
		if err != nil {
			logger.Errorf("Failed to reconcile Application: %q failed to reconcile PersistentVolumeClaim: %q; %v", application.Name, buildCache, zap.Error(err))
			return err
		}
		if buildCache == nil {
			c.Recorder.Eventf(application, corev1.EventTypeNormal, "Deleted", "Deleted PersistentVolumeClaim %q", buildCacheName)
		}
	}

	// Update our Status based on the state of our underlying PersistentVolumeClaim.
	if buildCache == nil {
		application.Status.MarkBuildCacheNotUsed()
	} else {
		application.Status.BuildCacheName = buildCache.Name
		application.Status.PropagateBuildCacheStatus(&buildCache.Status)
	}

	targetImage, err := c.resolveTargetImage(application)
	if err != nil {
		if err == errMissingDefaultPrefix {
			application.Status.MarkImageDefaultPrefixMissing(err.Error())
		} else {
			application.Status.MarkImageInvalid(err.Error())
		}
		return err
	}
	application.Status.TargetImage = targetImage

	buildName := resourcenames.Build(application)
	build, err := c.knbuildLister.Builds(application.Namespace).Get(buildName)
	if errors.IsNotFound(err) {
		build, err = c.createBuild(application)
		if err != nil {
			logger.Errorf("Failed to create Build %q: %v", buildName, err)
			c.Recorder.Eventf(application, corev1.EventTypeWarning, "CreationFailed", "Failed to create Build %q: %v", buildName, err)
			return err
		}
		if build != nil {
			c.Recorder.Eventf(application, corev1.EventTypeNormal, "Created", "Created Build %q", buildName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile Application: %q failed to Get Build: %q; %v", application.Name, buildName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(build, application) {
		// Surface an error in the application's status,and return an error.
		application.Status.MarkBuildNotOwned(buildName)
		return fmt.Errorf("Application: %q does not own Build: %q", application.Name, buildName)
	} else if build, err = c.reconcileBuild(ctx, application, build); err != nil {
		logger.Errorf("Failed to reconcile Application: %q failed to reconcile Build: %q; %v", application.Name, build, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying Build.
	if build == nil {
		application.Status.MarkBuildNotUsed()
	} else {
		application.Status.BuildName = build.Name
		application.Status.PropagateBuildStatus(&build.Status)
	}
	if application.Status.GetCondition(buildv1alpha1.ApplicationConditionBuildSucceeded).IsTrue() {
		// resolve image name
		opt := k8schain.Options{
			Namespace: application.Namespace,
		}
		if build == nil {
			opt.ServiceAccountName = "riff-build"
		} else {
			opt.ServiceAccountName = build.Spec.ServiceAccountName
		}
		// TODO load from a configmap
		skipRegistries := sets.NewString()
		skipRegistries.Insert("ko.local")
		skipRegistries.Insert("dev.local")
		digest, err := c.resolver.Resolve(application.Status.TargetImage, opt, skipRegistries)
		if err != nil {
			application.Status.MarkImageMissing(fmt.Sprintf("Unable to fetch image %q: %s", application.Status.TargetImage, err.Error()))
			return err
		}
		application.Status.LatestImage = digest
		application.Status.MarkImageResolved()
	}

	application.Status.ObservedGeneration = application.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *buildv1alpha1.Application) (*buildv1alpha1.Application, error) {
	application, err := c.applicationLister.Applications(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(application.Status, desired.Status) {
		return application, nil
	}
	becomesReady := desired.Status.IsReady() && !application.Status.IsReady()
	// Don't modify the informers copy.
	existing := application.DeepCopy()
	existing.Status = desired.Status

	f, err := c.ProjectriffClientSet.BuildV1alpha1().Applications(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(f.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("Application %q became ready after %v", application.Name, duration)
	}

	return f, err
}

func (c *Reconciler) resolveTargetImage(application *buildv1alpha1.Application) (string, error) {
	if !strings.HasPrefix(application.Spec.Image, "_") {
		return application.Spec.Image, nil
	}
	riffBuildConfig, err := c.configmapLister.ConfigMaps(application.Namespace).Get("riff-build")
	if err != nil {
		if apierrs.IsNotFound(err) {
			return "", errMissingDefaultPrefix
		}
		return "", err
	}
	defaultPrefix := riffBuildConfig.Data["default-image-prefix"]
	if defaultPrefix == "" {
		return "", errMissingDefaultPrefix
	}
	image, err := buildv1alpha1.ResolveDefaultImage(application, defaultPrefix)
	if err != nil {
		return "", err
	}
	return image, nil
}

func (c *Reconciler) reconcileBuildCache(ctx context.Context, application *buildv1alpha1.Application, buildCache *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	logger := logging.FromContext(ctx)
	desiredBuildCache, err := resources.MakeBuildCache(application)
	if err != nil {
		return nil, err
	}

	if desiredBuildCache == nil {
		// drop an existing buildCache
		return nil, c.KubeClientSet.CoreV1().PersistentVolumeClaims(application.Namespace).Delete(buildCache.Name, &metav1.DeleteOptions{
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
	return c.KubeClientSet.CoreV1().PersistentVolumeClaims(application.Namespace).Update(existing)
}

func (c *Reconciler) createBuildCache(application *buildv1alpha1.Application) (*corev1.PersistentVolumeClaim, error) {
	buildCache, err := resources.MakeBuildCache(application)
	if err != nil {
		return nil, err
	}
	if buildCache == nil {
		// nothing to create
		return buildCache, nil
	}
	return c.KubeClientSet.CoreV1().PersistentVolumeClaims(application.Namespace).Create(buildCache)
}

func buildCacheSemanticEquals(desiredBuildCache, buildCache *corev1.PersistentVolumeClaim) bool {
	return equality.Semantic.DeepEqual(desiredBuildCache.Spec.Resources, buildCache.Spec.Resources) &&
		equality.Semantic.DeepEqual(desiredBuildCache.ObjectMeta.Labels, buildCache.ObjectMeta.Labels)
}

func (c *Reconciler) reconcileBuild(ctx context.Context, application *buildv1alpha1.Application, build *knbuildv1alpha1.Build) (*knbuildv1alpha1.Build, error) {
	logger := logging.FromContext(ctx)
	desiredBuild, err := resources.MakeBuild(application)
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
	return c.KnBuildClientSet.BuildV1alpha1().Builds(application.Namespace).Update(existing)
}

func (c *Reconciler) createBuild(application *buildv1alpha1.Application) (*knbuildv1alpha1.Build, error) {
	build, err := resources.MakeBuild(application)
	if err != nil {
		return nil, err
	}
	if build == nil {
		// nothing to create
		return build, nil
	}
	return c.KnBuildClientSet.BuildV1alpha1().Builds(application.Namespace).Create(build)
}

func buildSemanticEquals(desiredBuild, build *knbuildv1alpha1.Build) bool {
	return equality.Semantic.DeepEqual(desiredBuild.Spec, build.Spec) &&
		equality.Semantic.DeepEqual(desiredBuild.ObjectMeta.Labels, build.ObjectMeta.Labels)
}
