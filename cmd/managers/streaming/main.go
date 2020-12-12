/*
Copyright 2019 the original author or authors.

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
	"net/http"
	"os"
	"time"

	"github.com/vmware-labs/reconciler-runtime/reconcilers"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
	kedav1alpha1 "github.com/projectriff/system/pkg/apis/thirdparty/keda/v1alpha1"
	streamingcontrollers "github.com/projectriff/system/pkg/controllers/streaming"
	// +kubebuilder:scaffold:imports
)

var (
	scheme     = runtime.NewScheme()
	setupLog   = ctrl.Log.WithName("setup")
	syncPeriod = 10 * time.Hour
	namespace  = os.Getenv("SYSTEM_NAMESPACE")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = buildv1alpha1.AddToScheme(scheme)
	_ = kedav1alpha1.AddToScheme(scheme)

	_ = streamingv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	ctx := ctrl.SetupSignalHandler()

	var metricsAddr string
	var probesAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probesAddr, "probes-addr", ":8081", "The address health probes bind to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New())

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		HealthProbeBindAddress: probesAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "controller-leader-election-helper-streaming",
		SyncPeriod:             &syncPeriod,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	streamControllerLogger := ctrl.Log.WithName("controllers").WithName("Stream")
	if err = streamingcontrollers.StreamReconciler(
		reconcilers.NewConfig(mgr, &streamingv1alpha1.Stream{}, syncPeriod),
		streamingcontrollers.NewStreamProvisionerClient(http.DefaultClient, streamControllerLogger),
	).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Stream")
		os.Exit(1)
	}
	if err = ctrl.NewWebhookManagedBy(mgr).For(&streamingv1alpha1.Stream{}).Complete(); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Stream")
		os.Exit(1)
	}
	if err = streamingcontrollers.ProcessorReconciler(
		reconcilers.NewConfig(mgr, &streamingv1alpha1.Processor{}, syncPeriod),
		namespace,
	).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Processor")
		os.Exit(1)
	}
	if err = ctrl.NewWebhookManagedBy(mgr).For(&streamingv1alpha1.Processor{}).Complete(); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Processor")
		os.Exit(1)
	}
	if err = streamingcontrollers.GatewayReconciler(
		reconcilers.NewConfig(mgr, &streamingv1alpha1.Gateway{}, syncPeriod),
	).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Gateway")
		os.Exit(1)
	}
	if err = ctrl.NewWebhookManagedBy(mgr).For(&streamingv1alpha1.Gateway{}).Complete(); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Gateway")
		os.Exit(1)
	}
	if err = streamingcontrollers.KafkaGatewayReconciler(
		reconcilers.NewConfig(mgr, &streamingv1alpha1.KafkaGateway{}, syncPeriod),
		namespace,
	).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KafkaGateway")
		os.Exit(1)
	}
	if err = ctrl.NewWebhookManagedBy(mgr).For(&streamingv1alpha1.KafkaGateway{}).Complete(); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "KafkaGateway")
		os.Exit(1)
	}
	if err = streamingcontrollers.PulsarGatewayReconciler(
		reconcilers.NewConfig(mgr, &streamingv1alpha1.PulsarGateway{}, syncPeriod),
		namespace,
	).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PulsarGateway")
		os.Exit(1)
	}
	if err = ctrl.NewWebhookManagedBy(mgr).For(&streamingv1alpha1.PulsarGateway{}).Complete(); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "PulsarGateway")
		os.Exit(1)
	}
	if err = streamingcontrollers.InMemoryGatewayReconciler(
		reconcilers.NewConfig(mgr, &streamingv1alpha1.InMemoryGateway{}, syncPeriod),
		namespace,
	).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "InMemoryGateway")
		os.Exit(1)
	}
	if err = ctrl.NewWebhookManagedBy(mgr).For(&streamingv1alpha1.InMemoryGateway{}).Complete(); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "InMemoryGateway")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("default", func(_ *http.Request) error { return nil }); err != nil {
		setupLog.Error(err, "unable to create health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("default", func(_ *http.Request) error { return nil }); err != nil {
		setupLog.Error(err, "unable to create ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
