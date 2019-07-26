/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconciler

import (
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	kedaclientset "github.com/kedacore/keda/pkg/client/clientset/versioned"
	knbuildclientset "github.com/knative/build/pkg/client/clientset/versioned"
	"github.com/knative/pkg/configmap"
	"github.com/knative/pkg/logging/logkey"
	knservingclientset "github.com/knative/serving/pkg/client/clientset/versioned"
	projectriffclientset "github.com/projectriff/system/pkg/client/clientset/versioned"
	projectriffScheme "github.com/projectriff/system/pkg/client/clientset/versioned/scheme"
)

// Options defines the common reconciler options.
// We define this to reduce the boilerplate argument list when
// creating our controllers.
type Options struct {
	KubeClientSet        kubernetes.Interface
	ProjectriffClientSet projectriffclientset.Interface
	KnBuildClientSet     knbuildclientset.Interface
	KnServingClientSet   knservingclientset.Interface
	KedaClientSet        kedaclientset.Interface
	Recorder             record.EventRecorder

	ConfigMapWatcher configmap.Watcher
	Logger           *zap.SugaredLogger

	ResyncPeriod time.Duration
	StopChannel  <-chan struct{}
}

// GetTrackerLease returns a multiple of the resync period to use as the
// duration for tracker leases. This attempts to ensure that resyncs happen to
// refresh leases frequently enough that we don't miss updates to tracked
// objects.
func (o Options) GetTrackerLease() time.Duration {
	return o.ResyncPeriod * 3
}

// Base implements the core controller logic, given a Reconciler.
type Base struct {
	// KubeClientSet allows us to talk to the k8s for core APIs
	KubeClientSet kubernetes.Interface

	// ProjectriffClientSet allows us to configure projectriff objects
	ProjectriffClientSet projectriffclientset.Interface

	// KnBuildClientSet allows us to configure Knative build objects
	KnBuildClientSet knbuildclientset.Interface

	// KnServingClientSet allows us to configure Knative serving objects
	KnServingClientSet knservingclientset.Interface

	// KedaClientSet allows us to configure Keda scaledObjects
	KedaClientSet kedaclientset.Interface

	// ConfigMapWatcher allows us to watch for ConfigMap changes.
	ConfigMapWatcher configmap.Watcher

	// Recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	Recorder record.EventRecorder

	// Sugared logger is easier to use but is not as performant as the
	// raw logger. In performance critical paths, call logger.Desugar()
	// and use the returned raw logger instead. In addition to the
	// performance benefits, raw logger also preserves type-safety at
	// the expense of slightly greater verbosity.
	Logger *zap.SugaredLogger
}

// NewBase instantiates a new instance of Base implementing
// the common & boilerplate code between our reconcilers.
func NewBase(opt Options, controllerAgentName string) *Base {
	// Enrich the logs with controller name
	logger := opt.Logger.Named(controllerAgentName).With(zap.String(logkey.ControllerType, controllerAgentName))

	recorder := opt.Recorder
	if recorder == nil {
		// Create event broadcaster
		logger.Debug("Creating event broadcaster")
		eventBroadcaster := record.NewBroadcaster()
		eventBroadcaster.StartLogging(logger.Named("event-broadcaster").Infof)
		eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: opt.KubeClientSet.CoreV1().Events("")})
		recorder = eventBroadcaster.NewRecorder(
			scheme.Scheme, corev1.EventSource{Component: controllerAgentName})
	}

	base := &Base{
		KubeClientSet:        opt.KubeClientSet,
		ProjectriffClientSet: opt.ProjectriffClientSet,
		KnBuildClientSet:     opt.KnBuildClientSet,
		KnServingClientSet:   opt.KnServingClientSet,
		KedaClientSet:        opt.KedaClientSet,

		ConfigMapWatcher:     opt.ConfigMapWatcher,
		Recorder:             recorder,
		Logger:               logger,
	}

	return base
}

func init() {
	// Add projectriff types to the default Kubernetes Scheme so Events can be
	// logged for projectriff types.
	projectriffScheme.AddToScheme(scheme.Scheme)
}
