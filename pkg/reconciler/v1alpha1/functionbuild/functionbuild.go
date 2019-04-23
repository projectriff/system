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

package functionbuild

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/kmp"
	"github.com/knative/pkg/logging"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	buildinformers "github.com/projectriff/system/pkg/client/informers/externalversions/build/v1alpha1"
	buildlisters "github.com/projectriff/system/pkg/client/listers/build/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/functionbuild/resources"
	resourcenames "github.com/projectriff/system/pkg/reconciler/v1alpha1/functionbuild/resources/names"
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
	ReconcilerName      = "FunctionBuilds"
	controllerAgentName = "functionbuild-controller"
)

// Reconciler implements controller.Reconciler for FunctionBuild resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	functionbuildLister buildlisters.FunctionBuildLister
	internalbuildLister buildlisters.InternalBuildLister
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	functionbuildInformer buildinformers.FunctionBuildInformer,
	internalbuildInformer buildinformers.InternalBuildInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:                reconciler.NewBase(opt, controllerAgentName),
		functionbuildLister: functionbuildInformer.Lister(),
		internalbuildLister: internalbuildInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	functionbuildInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
		DeleteFunc: impl.Enqueue,
	})

	internalbuildInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(buildv1alpha1.SchemeGroupVersion.WithKind("FunctionBuild")),
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.EnqueueControllerOf,
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
			DeleteFunc: impl.EnqueueControllerOf,
		},
	})

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the FunctionBuild resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	// Get the FunctionBuild resource with this namespace/name
	original, err := c.functionbuildLister.FunctionBuilds(namespace).Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("functionbuild %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	functionbuild := original.DeepCopy()

	// Reconcile this copy of the functionbuild and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, functionbuild)

	if equality.Semantic.DeepEqual(original.Status, functionbuild.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.

	} else if _, uErr := c.updateStatus(functionbuild); uErr != nil {
		logger.Warn("Failed to update functionbuild status", zap.Error(uErr))
		c.Recorder.Eventf(functionbuild, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for FunctionBuild %q: %v", functionbuild.Name, uErr)
		return uErr
	} else if err == nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(functionbuild, corev1.EventTypeNormal, "Updated", "Updated FunctionBuild %q", functionbuild.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, functionbuild *buildv1alpha1.FunctionBuild) error {
	logger := logging.FromContext(ctx)
	if functionbuild.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	functionbuild.SetDefaults(context.Background())

	functionbuild.Status.InitializeConditions()

	internalbuildName := resourcenames.InternalBuild(functionbuild)
	internalbuild, err := c.internalbuildLister.InternalBuilds(functionbuild.Namespace).Get(internalbuildName)
	if errors.IsNotFound(err) {
		internalbuild, err = c.createInternalBuild(functionbuild)
		if err != nil {
			logger.Errorf("Failed to create InternalBuild %q: %v", internalbuildName, err)
			c.Recorder.Eventf(functionbuild, corev1.EventTypeWarning, "CreationFailed", "Failed to create InternalBuild %q: %v", internalbuildName, err)
			return err
		}
		if internalbuild != nil {
			c.Recorder.Eventf(functionbuild, corev1.EventTypeNormal, "Created", "Created InternalBuild %q", internalbuildName)
		}
	} else if err != nil {
		logger.Errorf("Failed to reconcile FunctionBuild: %q failed to Get InternalBuild: %q; %v", functionbuild.Name, internalbuildName, zap.Error(err))
		return err
	} else if !metav1.IsControlledBy(internalbuild, functionbuild) {
		// Surface an error in the functionbuild's status,and return an error.
		functionbuild.Status.MarkInternalBuildNotOwned(internalbuildName)
		return fmt.Errorf("FunctionBuild: %q does not own InternalBuild: %q", functionbuild.Name, internalbuildName)
	} else if internalbuild, err = c.reconcileInternalBuild(ctx, functionbuild, internalbuild); err != nil {
		logger.Errorf("Failed to reconcile FunctionBuild: %q failed to reconcile InternalBuild: %q; %v", functionbuild.Name, internalbuild, zap.Error(err))
		return err
	}

	// Update our Status based on the state of our underlying InternalBuild.
	functionbuild.Status.InternalBuildName = internalbuild.Name
	functionbuild.Status.PropagateInternalBuildStatus(&internalbuild.Status)

	functionbuild.Status.ObservedGeneration = functionbuild.Generation

	return nil
}

func (c *Reconciler) updateStatus(desired *buildv1alpha1.FunctionBuild) (*buildv1alpha1.FunctionBuild, error) {
	functionbuild, err := c.functionbuildLister.FunctionBuilds(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(functionbuild.Status, desired.Status) {
		return functionbuild, nil
	}
	becomesReady := desired.Status.IsReady() && !functionbuild.Status.IsReady()
	// Don't modify the informers copy.
	existing := functionbuild.DeepCopy()
	existing.Status = desired.Status

	fb, err := c.ProjectriffClientSet.BuildV1alpha1().FunctionBuilds(desired.Namespace).UpdateStatus(existing)
	if err == nil && becomesReady {
		duration := time.Now().Sub(fb.ObjectMeta.CreationTimestamp.Time)
		c.Logger.Infof("FunctionBuild %q became ready after %v", functionbuild.Name, duration)
	}

	return fb, err
}

func (c *Reconciler) reconcileInternalBuild(ctx context.Context, functionbuild *buildv1alpha1.FunctionBuild, internalbuild *buildv1alpha1.InternalBuild) (*buildv1alpha1.InternalBuild, error) {
	logger := logging.FromContext(ctx)
	desiredInternalbuild, err := resources.MakeInternalBuild(functionbuild)
	if err != nil {
		return nil, err
	}

	if buildSemanticEquals(desiredInternalbuild, internalbuild) {
		// No differences to reconcile.
		return internalbuild, nil
	}
	diff, err := kmp.SafeDiff(desiredInternalbuild.Spec, internalbuild.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff InternalBuild: %v", err)
	}
	logger.Infof("Reconciling internalbuild diff (-desired, +observed): %s", diff)

	// Don't modify the informers copy.
	existing := internalbuild.DeepCopy()
	// Preserve the rest of the object (e.g. ObjectMeta except for labels).
	existing.Spec = desiredInternalbuild.Spec
	existing.ObjectMeta.Labels = desiredInternalbuild.ObjectMeta.Labels
	return c.ProjectriffClientSet.BuildV1alpha1().InternalBuilds(functionbuild.Namespace).Update(existing)
}

func (c *Reconciler) createInternalBuild(functionbuild *buildv1alpha1.FunctionBuild) (*buildv1alpha1.InternalBuild, error) {
	internalbuild, err := resources.MakeInternalBuild(functionbuild)
	if err != nil {
		return nil, err
	}
	if internalbuild == nil {
		// nothing to create
		return internalbuild, nil
	}
	return c.ProjectriffClientSet.BuildV1alpha1().InternalBuilds(functionbuild.Namespace).Create(internalbuild)
}

func buildSemanticEquals(desiredInternalbuild, internalbuild *buildv1alpha1.InternalBuild) bool {
	return equality.Semantic.DeepEqual(desiredInternalbuild.Spec, internalbuild.Spec) &&
		equality.Semantic.DeepEqual(desiredInternalbuild.ObjectMeta.Labels, internalbuild.ObjectMeta.Labels)
}
