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

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	kedav1alpha1 "github.com/projectriff/system/pkg/apis/thirdparty/keda/v1alpha1"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
	controllers "github.com/projectriff/system/pkg/controllers/streaming"
	"github.com/projectriff/system/pkg/tracker"
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
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.Logger(true))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "controller-leader-election-helper-streaming",
		SyncPeriod:         &syncPeriod,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	streamControllerLogger := ctrl.Log.WithName("controllers").WithName("Stream")
	if err = (&controllers.KafkaProviderReconciler{
		Client:    mgr.GetClient(),
		Log:       ctrl.Log.WithName("controllers").WithName("KafkaProvider"),
		Scheme:    mgr.GetScheme(),
		Tracker:   tracker.New(syncPeriod),
		Namespace: namespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KafkaProvider")
		os.Exit(1)
	}
	if err = (&controllers.StreamReconciler{
		Client:                  mgr.GetClient(),
		Log:                     streamControllerLogger,
		Scheme:                  mgr.GetScheme(),
		StreamProvisionerClient: controllers.NewStreamProvisionerClient(http.DefaultClient, streamControllerLogger),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Stream")
		os.Exit(1)
	}
	if err = (&controllers.ProcessorReconciler{
		Client:    mgr.GetClient(),
		Log:       ctrl.Log.WithName("controllers").WithName("Processor"),
		Scheme:    mgr.GetScheme(),
		Tracker:   tracker.New(syncPeriod),
		Namespace: namespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Processor")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
