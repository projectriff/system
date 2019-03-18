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

package main

import (
	"flag"
	"log"
	"time"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/knative/pkg/configmap"
	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/signals"
	"github.com/knative/pkg/system"
	"github.com/knative/pkg/version"
	servingclientset "github.com/knative/serving/pkg/client/clientset/versioned"
	servinginformers "github.com/knative/serving/pkg/client/informers/externalversions"
	projectriffclientset "github.com/projectriff/system/pkg/client/clientset/versioned"
	projectriffinformers "github.com/projectriff/system/pkg/client/informers/externalversions"
	"github.com/projectriff/system/pkg/logging"
	"github.com/projectriff/system/pkg/metrics"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/application"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/function"
	"go.uber.org/zap"
)

const (
	threadsPerController = 2
	component            = "controller"
)

var (
	masterURL  = flag.String("master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	kubeconfig = flag.String("kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
)

func main() {
	flag.Parse()
	loggingConfigMap, err := configmap.Load("/etc/config-logging")
	if err != nil {
		log.Fatalf("Error loading logging configuration: %v", err)
	}
	loggingConfig, err := logging.NewConfigFromMap(loggingConfigMap)
	if err != nil {
		log.Fatalf("Error parsing logging configuration: %v", err)
	}
	logger, atomicLevel := logging.NewLoggerFromConfig(loggingConfig, component)
	defer logger.Sync()

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(*masterURL, *kubeconfig)
	if err != nil {
		logger.Fatalw("Error building kubeconfig", zap.Error(err))
	}

	// We run 6 controllers, so bump the defaults.
	cfg.QPS = 6 * rest.DefaultQPS
	cfg.Burst = 6 * rest.DefaultBurst

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logger.Fatalw("Error building kubernetes clientset", zap.Error(err))
	}

	projectriffClient, err := projectriffclientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalw("Error building projectriff clientset", zap.Error(err))
	}

	servingClient, err := servingclientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalw("Error building serving clientset", zap.Error(err))
	}

	if err := version.CheckMinimumVersion(kubeClient.Discovery()); err != nil {
		logger.Fatalf("Version check failed: %v", err)
	}

	configMapWatcher := configmap.NewInformedWatcher(kubeClient, system.Namespace())

	opt := reconciler.Options{
		KubeClientSet:        kubeClient,
		ProjectriffClientSet: projectriffClient,
		ServingClientSet:     servingClient,
		ConfigMapWatcher:     configMapWatcher,
		Logger:               logger,
		ResyncPeriod:         10 * time.Hour, // Based on controller-runtime default.
		StopChannel:          stopCh,
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, opt.ResyncPeriod)
	projectriffInformerFactory := projectriffinformers.NewSharedInformerFactory(projectriffClient, opt.ResyncPeriod)
	servingInformerFactory := servinginformers.NewSharedInformerFactory(servingClient, opt.ResyncPeriod)

	applicationInformer := projectriffInformerFactory.Projectriff().V1alpha1().Applications()
	functionInformer := projectriffInformerFactory.Projectriff().V1alpha1().Functions()
	serviceInformer := servingInformerFactory.Serving().V1alpha1().Services()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims()

	// Build all of our controllers, with the clients constructed above.
	// Add new controllers to this array.
	controllers := []*controller.Impl{
		application.NewController(
			opt,
			applicationInformer,
			serviceInformer,
			pvcInformer,
		),
		function.NewController(
			opt,
			functionInformer,
			applicationInformer,
		),
	}

	// Watch the logging config map and dynamically update logging levels.
	configMapWatcher.Watch(logging.ConfigName, logging.UpdateLevelFromConfigMap(logger, atomicLevel, component))
	// Watch the observability config map and dynamically update metrics exporter.
	configMapWatcher.Watch(metrics.ObservabilityConfigName, metrics.UpdateExporterFromConfigMap(component, logger))

	// These are non-blocking.
	kubeInformerFactory.Start(stopCh)
	projectriffInformerFactory.Start(stopCh)
	servingInformerFactory.Start(stopCh)
	if err := configMapWatcher.Start(stopCh); err != nil {
		logger.Fatalw("failed to start configuration manager", zap.Error(err))
	}

	// Wait for the caches to be synced before starting controllers.
	logger.Info("Waiting for informer caches to sync")
	for i, synced := range []cache.InformerSynced{
		applicationInformer.Informer().HasSynced,
		functionInformer.Informer().HasSynced,
		serviceInformer.Informer().HasSynced,
	} {
		if ok := cache.WaitForCacheSync(stopCh, synced); !ok {
			logger.Fatalf("Failed to wait for cache at index %d to sync", i)
		}
	}

	// Start all of the controllers.
	for _, ctrlr := range controllers {
		go func(ctrlr *controller.Impl) {
			// We don't expect this to return until stop is called,
			// but if it does, propagate it back.
			if runErr := ctrlr.Run(threadsPerController, stopCh); runErr != nil {
				logger.Fatalw("Error running controller", zap.Error(runErr))
			}
		}(ctrlr)
	}

	<-stopCh
}
