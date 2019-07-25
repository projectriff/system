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

package container

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn/k8schain"
	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/logging"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	buildinformers "github.com/projectriff/system/pkg/client/informers/externalversions/build/v1alpha1"
	buildlisters "github.com/projectriff/system/pkg/client/listers/build/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/digest"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	coreinformers "k8s.io/client-go/informers/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	// ReconcilerName is the name of the reconciler
	ReconcilerName      = "Containers"
	controllerAgentName = "container-controller"
)

var errMissingDefaultPrefix = fmt.Errorf("missing default image prefix")

// Reconciler implements controller.Reconciler for Container resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	containerLister buildlisters.ContainerLister
	configmapLister corelisters.ConfigMapLister

	resolver digest.Resolver
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	containerInformer buildinformers.ContainerInformer,
	configmapInformer coreinformers.ConfigMapInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:            reconciler.NewBase(opt, controllerAgentName),
		containerLister: containerInformer.Lister(),
		configmapLister: configmapInformer.Lister(),

		resolver: digest.NewDefaultResolver(opt),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName)

	c.Logger.Info("Setting up event handlers")
	containerInformer.Informer().AddEventHandler(reconciler.Handler(impl.Enqueue))

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Container resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the Container resource with this namespace/name
	original, err := c.containerLister.Containers(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("container %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	container := original.DeepCopy()

	// Reconcile this copy of the container and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, container)

	if equality.Semantic.DeepEqual(original.Status, container.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(container); uErr != nil {
		logger.Warn("Failed to update container status", zap.Error(uErr))
		c.Recorder.Eventf(container, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for Container %q: %v", container.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(container, corev1.EventTypeNormal, "Updated", "Updated Container %q", container.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, container *buildv1alpha1.Container) error {
	if container.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	container.SetDefaults(context.Background())

	container.Status.InitializeConditions()

	targetImage, err := c.resolveTargetImage(container)
	if err != nil {
		if err == errMissingDefaultPrefix {
			container.Status.MarkImageDefaultPrefixMissing(err.Error())
		} else {
			container.Status.MarkImageInvalid(err.Error())
		}
		return err
	}
	container.Status.TargetImage = targetImage

	// resolve image name
	opt := k8schain.Options{
		Namespace:          container.Namespace,
		ServiceAccountName: "riff-build",
	}
	// TODO load from a configmap
	skipRegistries := sets.NewString()
	skipRegistries.Insert("ko.local")
	skipRegistries.Insert("dev.local")
	digest, err := c.resolver.Resolve(container.Status.TargetImage, opt, skipRegistries)
	if err != nil {
		container.Status.MarkImageMissing(fmt.Sprintf("Unable to fetch image %q: %s", container.Status.TargetImage, err.Error()))
		return err
	}
	container.Status.LatestImage = digest
	container.Status.MarkImageResolved()

	container.Status.ObservedGeneration = container.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *buildv1alpha1.Container) (*buildv1alpha1.Container, error) {
	container, err := c.containerLister.Containers(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(container.Status, desired.Status) {
		return container, nil
	}
	becomesReady := desired.Status.IsReady() && !container.Status.IsReady()
	// Don't modify the informers copy.
	existing := container.DeepCopy()
	existing.Status = desired.Status

	f, err := c.ProjectriffClientSet.BuildV1alpha1().Containers(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(f.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("Container %q became ready after %v", container.Name, duration)
	}

	return f, err
}

func (c *Reconciler) resolveTargetImage(container *buildv1alpha1.Container) (string, error) {
	if !strings.HasPrefix(container.Spec.Image, "_") {
		return container.Spec.Image, nil
	}
	riffBuildConfig, err := c.configmapLister.ConfigMaps(container.Namespace).Get("riff-build")
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
	image, err := buildv1alpha1.ResolveDefaultImage(container, defaultPrefix)
	if err != nil {
		return "", err
	}
	return image, nil
}
