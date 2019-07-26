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

	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	"k8s.io/apimachinery/pkg/api/errors"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	kedaclientset "github.com/kedacore/keda/pkg/client/clientset/versioned"
	kedainformers "github.com/kedacore/keda/pkg/client/informers/externalversions"
	knbuildclientset "github.com/knative/build/pkg/client/clientset/versioned"
	knbuildinformers "github.com/knative/build/pkg/client/informers/externalversions"
	"github.com/knative/pkg/configmap"
	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/signals"
	"github.com/knative/pkg/system"
	"github.com/knative/pkg/version"
	knservingclientset "github.com/knative/serving/pkg/client/clientset/versioned"
	knservinginformers "github.com/knative/serving/pkg/client/informers/externalversions"
	projectriffclientset "github.com/projectriff/system/pkg/client/clientset/versioned"
	projectriffinformers "github.com/projectriff/system/pkg/client/informers/externalversions"
	"github.com/projectriff/system/pkg/logging"
	"github.com/projectriff/system/pkg/metrics"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/application"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/builder"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/container"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/coredeployer"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/credential"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/function"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/knativeadapter"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/knativedeployer"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/streamingprocessor"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/streamingstream"
	"go.uber.org/zap"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
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

	var knativeRuntime = false
	var streamingRuntime = false

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

	knbuildClient, err := knbuildclientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalw("Error building knbuild clientset", zap.Error(err))
	}

	knservingClient, err := knservingclientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalw("Error building knserving clientset", zap.Error(err))
	}

	kedaClient, err := kedaclientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalw("Error building keda clientset", zap.Error(err))
	}

	if err := version.CheckMinimumVersion(kubeClient.Discovery()); err != nil {
		logger.Fatalf("Version check failed: %v", err)
	}

	configMapWatcher := configmap.NewInformedWatcher(kubeClient, system.Namespace())

	knativeRuntime = detectKnativeRuntime(cfg, logger)
	if knativeRuntime {
		logger.Info("Starting with Knative runtime")
	} else {
		logger.Warn("Starting without Knative runtime")
	}
	streamingRuntime = detectStreamingRuntime(cfg, logger)
	if knativeRuntime {
		logger.Info("Starting with Streaming runtime")
	} else {
		logger.Warn("Starting without Streaming runtime")
	}

	opt := reconciler.Options{
		KubeClientSet:        kubeClient,
		ProjectriffClientSet: projectriffClient,
		KnBuildClientSet:     knbuildClient,
		KnServingClientSet:   knservingClient,
		KedaClientSet:        kedaClient,

		ConfigMapWatcher: configMapWatcher,
		Logger:           logger,
		ResyncPeriod:     10 * time.Hour, // Based on controller-runtime default.
		StopChannel:      stopCh,
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, opt.ResyncPeriod)
	projectriffInformerFactory := projectriffinformers.NewSharedInformerFactory(projectriffClient, opt.ResyncPeriod)
	knbuildInformerFactory := knbuildinformers.NewSharedInformerFactory(knbuildClient, opt.ResyncPeriod)
	knservingInformerFactory := knservinginformers.NewSharedInformerFactory(knservingClient, opt.ResyncPeriod)
	kedaInformerFactory := kedainformers.NewSharedInformerFactory(kedaClient, opt.ResyncPeriod)

	applicationInformer := projectriffInformerFactory.Build().V1alpha1().Applications()
	containerInformer := projectriffInformerFactory.Build().V1alpha1().Containers()
	functionInformer := projectriffInformerFactory.Build().V1alpha1().Functions()
	coredeployerInformer := projectriffInformerFactory.Core().V1alpha1().Deployers()
	streamInformer := projectriffInformerFactory.Streaming().V1alpha1().Streams()
	processorInformer := projectriffInformerFactory.Streaming().V1alpha1().Processors()
	knativeadapterInformer := projectriffInformerFactory.Knative().V1alpha1().Adapters()
	knativedeployerInformer := projectriffInformerFactory.Knative().V1alpha1().Deployers()

	deploymentInformer := kubeInformerFactory.Apps().V1().Deployments()
	serviceInformer := kubeInformerFactory.Core().V1().Services()
	configmapInformer := kubeInformerFactory.Core().V1().ConfigMaps()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims()
	secretInformer := kubeInformerFactory.Core().V1().Secrets()
	serviceaccountInformer := kubeInformerFactory.Core().V1().ServiceAccounts()
	knbuildInformer := knbuildInformerFactory.Build().V1alpha1().Builds()
	knclusterbuildtemplateInformer := knbuildInformerFactory.Build().V1alpha1().ClusterBuildTemplates()
	knserviceInformer := knservingInformerFactory.Serving().V1alpha1().Services()
	knconfigurationInformer := knservingInformerFactory.Serving().V1alpha1().Configurations()
	knrouteInformer := knservingInformerFactory.Serving().V1alpha1().Routes()
	scaledObjectInformer := kedaInformerFactory.Keda().V1alpha1().ScaledObjects()

	// Build all of our controllers, with the clients constructed above.
	// Add new controllers to this array.
	controllers := []*controller.Impl{
		// build.projectriff.io
		application.NewController(
			opt,
			applicationInformer,

			configmapInformer,
			pvcInformer,
			knbuildInformer,
		),
		container.NewController(
			opt,
			containerInformer,

			configmapInformer,
		),
		function.NewController(
			opt,
			functionInformer,

			configmapInformer,
			pvcInformer,
			knbuildInformer,
		),
		credential.NewController(
			opt,
			secretInformer,

			serviceaccountInformer,
			applicationInformer,
			functionInformer,
		),
		builder.NewController(
			opt,
			configmapInformer,

			knclusterbuildtemplateInformer,
		),
		// core.projectriff.io
		coredeployer.NewController(
			opt,
			coredeployerInformer,

			deploymentInformer,
			serviceInformer,
			applicationInformer,
			containerInformer,
			functionInformer,
		),
	}
	if knativeRuntime {
		controllers = append(controllers,
			// knative.projectriff.io
			knativeadapter.NewController(
				opt,
				knativeadapterInformer,

				applicationInformer,
				containerInformer,
				functionInformer,
				knserviceInformer,
				knconfigurationInformer,
			),
			knativedeployer.NewController(
				opt,
				knativedeployerInformer,

				knconfigurationInformer,
				knrouteInformer,
				applicationInformer,
				containerInformer,
				functionInformer,
			),
		)
	}
	if streamingRuntime {
		controllers = append(controllers,
			// streaming.projectriff.io
			streamingstream.NewController(
				opt,
				streamInformer,
			),
			streamingprocessor.NewController(
				opt,
				processorInformer,

				functionInformer,
				streamInformer,
				deploymentInformer,
				scaledObjectInformer,
			),
		)
	}

	// Watch the logging config map and dynamically update logging levels.
	configMapWatcher.Watch(logging.ConfigName, logging.UpdateLevelFromConfigMap(logger, atomicLevel, component))
	// Watch the observability config map and dynamically update metrics exporter.
	configMapWatcher.Watch(metrics.ObservabilityConfigName, metrics.UpdateExporterFromConfigMap(component, logger))

	// These are non-blocking.
	kubeInformerFactory.Start(stopCh)
	projectriffInformerFactory.Start(stopCh)
	knbuildInformerFactory.Start(stopCh)
	if knativeRuntime {
		knservingInformerFactory.Start(stopCh)
	}
	if streamingRuntime {
		kedaInformerFactory.Start(stopCh)
	}
	if err := configMapWatcher.Start(stopCh); err != nil {
		logger.Fatalw("failed to start configuration manager", zap.Error(err))
	}

	// Wait for the caches to be synced before starting controllers.
	logger.Info("Waiting for informer caches to sync")
	informersSynced := []cache.InformerSynced{
		applicationInformer.Informer().HasSynced,
		containerInformer.Informer().HasSynced,
		functionInformer.Informer().HasSynced,
		coredeployerInformer.Informer().HasSynced,
		deploymentInformer.Informer().HasSynced,
		serviceInformer.Informer().HasSynced,
		pvcInformer.Informer().HasSynced,
		secretInformer.Informer().HasSynced,
		serviceaccountInformer.Informer().HasSynced,
		knbuildInformer.Informer().HasSynced,
	}
	if knativeRuntime {
		informersSynced = append(informersSynced,
			knativeadapterInformer.Informer().HasSynced,
			knativedeployerInformer.Informer().HasSynced,
			knserviceInformer.Informer().HasSynced,
			knconfigurationInformer.Informer().HasSynced,
			knrouteInformer.Informer().HasSynced,
		)
	}
	if streamingRuntime {
		informersSynced = append(informersSynced,
			streamInformer.Informer().HasSynced,
			processorInformer.Informer().HasSynced,
			scaledObjectInformer.Informer().HasSynced,
		)
	}
	for i, synced := range informersSynced {
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

func detectKnativeRuntime(cfg *rest.Config, logger *zap.SugaredLogger) bool {
	apiextensionsClient, err := apiextensionsclientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalw("Error building apiextensions clientset", zap.Error(err))
	}

	_, err = apiextensionsClient.ApiextensionsV1beta1().CustomResourceDefinitions().Get("revisions.serving.knative.dev", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false
		}
		logger.Fatalw("Error resolving Knative runtime", zap.Error(err))
	}

	return true
}

// detectStreamingRuntime detects whether keda is available, as it is a required dependency for supporting streaming.
func detectStreamingRuntime(cfg *rest.Config, logger *zap.SugaredLogger) bool {
	apiextensionsClient, err := apiextensionsclientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalw("Error building apiextensions clientset", zap.Error(err))
	}

	_, err = apiextensionsClient.ApiextensionsV1beta1().CustomResourceDefinitions().Get("scaledobjects.keda.k8s.io", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false
		}
		logger.Fatalw("Error resolving Streaming runtime", zap.Error(err))
	}

	return true
}
