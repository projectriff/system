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

	"k8s.io/client-go/tools/clientcmd"

	"go.uber.org/zap"

	"github.com/knative/pkg/configmap"
	"github.com/knative/pkg/logging/logkey"
	"github.com/knative/pkg/signals"
	"github.com/knative/pkg/system"
	"github.com/knative/pkg/version"
	"github.com/knative/pkg/webhook"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	requestv1alpha1 "github.com/projectriff/system/pkg/apis/request/v1alpha1"
	streamv1alpha1 "github.com/projectriff/system/pkg/apis/stream/v1alpha1"
	"github.com/projectriff/system/pkg/logging"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

const (
	component = "webhook"
)

var (
	masterURL  = flag.String("master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	kubeconfig = flag.String("kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
)

func main() {
	flag.Parse()
	cm, err := configmap.Load("/etc/config-logging")
	if err != nil {
		log.Fatalf("Error loading logging configuration: %v", err)
	}
	config, err := logging.NewConfigFromMap(cm)
	if err != nil {
		log.Fatalf("Error parsing logging configuration: %v", err)
	}
	logger, atomicLevel := logging.NewLoggerFromConfig(config, component)
	defer logger.Sync()
	logger = logger.With(zap.String(logkey.ControllerType, component))

	logger.Info("Starting the Configuration Webhook")

	// Set up signals so we handle the first shutdown signal gracefully.
	stopCh := signals.SetupSignalHandler()

	clusterConfig, err := clientcmd.BuildConfigFromFlags(*masterURL, *kubeconfig)
	if err != nil {
		logger.Fatalw("Failed to get cluster config", zap.Error(err))
	}

	kubeClient, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		logger.Fatalw("Failed to get the client set", zap.Error(err))
	}

	if err := version.CheckMinimumVersion(kubeClient.Discovery()); err != nil {
		logger.Fatalf("Version check failed: %v", err)
	}

	// Watch the logging config map and dynamically update logging levels.
	configMapWatcher := configmap.NewInformedWatcher(kubeClient, system.Namespace())
	configMapWatcher.Watch(logging.ConfigName, logging.UpdateLevelFromConfigMap(logger, atomicLevel, component))
	if err = configMapWatcher.Start(stopCh); err != nil {
		logger.Fatalw("Failed to start the ConfigMap watcher", zap.Error(err))
	}

	options := webhook.ControllerOptions{
		ServiceName:    "webhook",
		DeploymentName: "webhook",
		Namespace:      system.Namespace(),
		Port:           443,
		SecretName:     "webhook-certs",
		WebhookName:    "webhook.system.projectriff.io",
	}
	controller := webhook.AdmissionController{
		Client:  kubeClient,
		Options: options,
		Handlers: map[schema.GroupVersionKind]webhook.GenericCRD{
			buildv1alpha1.SchemeGroupVersion.WithKind("Application"): &buildv1alpha1.Application{},
			buildv1alpha1.SchemeGroupVersion.WithKind("Container"):   &buildv1alpha1.Container{},
			buildv1alpha1.SchemeGroupVersion.WithKind("Function"):    &buildv1alpha1.Function{},
			requestv1alpha1.SchemeGroupVersion.WithKind("Handler"):   &requestv1alpha1.Handler{},
			streamv1alpha1.SchemeGroupVersion.WithKind("Stream"):     &streamv1alpha1.Stream{},
			streamv1alpha1.SchemeGroupVersion.WithKind("Processor"):  &streamv1alpha1.Processor{},
		},
		Logger: logger,
	}
	if err = controller.Run(stopCh); err != nil {
		logger.Fatalw("Failed to start the admission controller", zap.Error(err))
	}
}
