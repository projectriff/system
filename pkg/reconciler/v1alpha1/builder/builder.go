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

package builder

import (
	"context"
	"fmt"

	knbuildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	knbuildinformers "github.com/knative/build/pkg/client/informers/externalversions/build/v1alpha1"
	knbuildlisters "github.com/knative/build/pkg/client/listers/build/v1alpha1"
	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/kmp"
	"github.com/knative/pkg/logging"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/digest"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	coreinformers "k8s.io/client-go/informers/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	// ReconcilerName is the name of the reconciler
	ReconcilerName      = "Builders"
	controllerAgentName = "builder-controller"

	buildersConfgMap = "builders"
)

// Reconciler implements controller.Reconciler for ConfigMap resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	configmapLister              corelisters.ConfigMapLister
	knclusterbuildtemplateLister knbuildlisters.ClusterBuildTemplateLister

	resolver digest.Resolver
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	configmapInformer coreinformers.ConfigMapInformer,
	knclusterbuildtemplateInformer knbuildinformers.ClusterBuildTemplateInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:                         reconciler.NewBase(opt, controllerAgentName),
		configmapLister:              configmapInformer.Lister(),
		knclusterbuildtemplateLister: knclusterbuildtemplateInformer.Lister(),

		resolver: digest.NewDefaultResolver(opt),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	knclusterbuildtemplateInformer.Informer().AddEventHandler(reconciler.Handler(func(obj interface{}) {
		impl.EnqueueKey("riff-system/" + buildersConfgMap)
	}))
	configmapInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			meta, err := meta.Accessor(obj)
			if err != nil {
				return false
			}
			return meta.GetNamespace() == "riff-system" && meta.GetName() == buildersConfgMap
		},
		Handler: reconciler.Handler(impl.Enqueue),
	})

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Data block of the ConfigMap resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}

	// Get the ConfigMap resource with this namespace/name
	original, err := c.configmapLister.ConfigMaps(namespace).Get(name)
	if err != nil && !apierrs.IsNotFound(err) {
		return err
	}

	// Don't modify the informers copy
	var configMap *corev1.ConfigMap
	if original != nil {
		configMap = original.DeepCopy()
	}

	// Reconcile this copy of the configMap
	configMap, err = c.reconcile(ctx, configMap)

	if err == nil && configMap != nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(configMap, corev1.EventTypeNormal, "Updated", "Reconciled ConfigMap %q", configMap.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, configMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	logger := logging.FromContext(ctx)
	if configMap != nil && configMap.GetDeletionTimestamp() != nil {
		return configMap, nil
	}

	buildTemplates, err := c.knclusterbuildtemplateLister.List(labels.Everything())
	if err != nil {
		logger.Errorf("Failed to reconcile ConfigMap: %q failed to Get ClusterBuildTemplates; %v", buildersConfgMap, zap.Error(err))
		return configMap, err
	}

	if configMap == nil {
		configMap, err := c.createConfigMap(buildTemplates)
		if err != nil {
			logger.Errorf("Failed to create ConfigMap %q: %v", buildersConfgMap, err)
			c.Recorder.Eventf(configMap, corev1.EventTypeWarning, "CreationFailed", "Failed to create ConfigMap %q: %v", buildersConfgMap, err)
			return configMap, err
		}
		if configMap != nil {
			c.Recorder.Eventf(configMap, corev1.EventTypeNormal, "Created", "Created ConfigMap %q", buildersConfgMap)
		}
	} else {
		configMap, err := c.reconcileConfigMap(ctx, configMap, buildTemplates)
		if err != nil {
			logger.Errorf("Failed to reconcile ConfigMap: %q; %v", buildersConfgMap, zap.Error(err))
			return configMap, err
		}
	}

	return configMap, nil
}

func (c *Reconciler) reconcileConfigMap(ctx context.Context, existingConfigMap *corev1.ConfigMap, buildTemplates []*knbuildv1alpha1.ClusterBuildTemplate) (*corev1.ConfigMap, error) {
	logger := logging.FromContext(ctx)

	configMap := existingConfigMap.DeepCopy()

	configMap.Data = map[string]string{}
	for _, buildTemplate := range buildTemplates {
		configMap.Data[buildTemplate.Name] = builderImage(buildTemplate.Spec)
	}

	if configMapSemanticEquals(configMap, existingConfigMap) {
		// No differences to reconcile.
		return configMap, nil
	}
	diff, err := kmp.SafeDiff(configMap.Data, existingConfigMap.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to diff ConfigMap: %v", err)
	}
	logger.Infof("Reconciling builders ConfigMap diff (-desired, +observed): %s", diff)

	return c.KubeClientSet.CoreV1().ConfigMaps(configMap.Namespace).Update(configMap)
}

func (c *Reconciler) createConfigMap(buildTemplates []*knbuildv1alpha1.ClusterBuildTemplate) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildersConfgMap,
			Namespace: "riff-system",
		},
		Data: map[string]string{},
	}
	for _, buildTemplate := range buildTemplates {
		configMap.Data[buildTemplate.Name] = builderImage(buildTemplate.Spec)
	}
	return c.KubeClientSet.CoreV1().ConfigMaps("riff-system").Create(configMap)
}

func configMapSemanticEquals(desiredConfigMap, configMap *corev1.ConfigMap) bool {
	return equality.Semantic.DeepEqual(desiredConfigMap.Data, configMap.Data)
}

func builderImage(buildTemplateSpec knbuildv1alpha1.BuildTemplateSpec) string {
	for _, p := range buildTemplateSpec.Parameters {
		if p.Name == "BUILDER_IMAGE" && p.Default != nil {
			return *p.Default
		}
	}
	return ""
}
