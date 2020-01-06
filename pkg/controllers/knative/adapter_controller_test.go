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

package knative_test

import (
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/projectriff/system/pkg/apis"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	knativev1alpha1 "github.com/projectriff/system/pkg/apis/knative/v1alpha1"
	knativeservingv1 "github.com/projectriff/system/pkg/apis/thirdparty/knative/serving/v1"
	"github.com/projectriff/system/pkg/controllers/knative"
	rtesting "github.com/projectriff/system/pkg/controllers/testing"
	"github.com/projectriff/system/pkg/controllers/testing/factories"
	"github.com/projectriff/system/pkg/tracker"
)

func TestAdapterReconcile(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-adapter"
	testKey := types.NamespacedName{Namespace: testNamespace, Name: testName}
	testImagePrefix := "example.com/repo"
	testSha256 := "cf8b4c69d5460f88530e1c80b8856a70801f31c50b191c8413043ba9b160a43e"
	testImage := fmt.Sprintf("%s/%s@sha256:%s", testImagePrefix, testName, testSha256)

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = buildv1alpha1.AddToScheme(scheme)
	_ = knativev1alpha1.AddToScheme(scheme)
	_ = knativeservingv1.AddToScheme(scheme)

	testAdapter := factories.AdapterKnative().
		NamespaceName(testNamespace, testName)

	testApplication := factories.Application().
		NamespaceName(testNamespace, "my-application")
	testFunction := factories.Function().
		NamespaceName(testNamespace, "my-function")
	testContainer := factories.Container().
		NamespaceName(testNamespace, "my-container")

	testConfiguration := factories.KnativeConfiguration().
		NamespaceName(testNamespace, "my-configuration").
		UserContainer(nil)
	testService := factories.KnativeService().
		NamespaceName(testNamespace, "my-service").
		UserContainer(nil)

	table := rtesting.Table{{
		Name: "adapter does not exist",
		Key:  testKey,
	}, {
		Name: "ignore deleted adapter",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testAdapter.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.Deleted(1)
				}).
				Get(),
		},
	}, {
		Name: "error fetching adapter",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "Adapter"),
		},
		GivenObjects: []runtime.Object{
			testAdapter.Get(),
		},
		ShouldErr: true,
	}, {
		Name: "error updating adapter status",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("update", "Adapter"),
		},
		GivenObjects: []runtime.Object{
			testAdapter.
				ApplicationRef(testApplication.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testApplication.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
			testService.Get(),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testApplication.Get(), testAdapter.Get(), scheme),
			rtesting.NewTrackRequest(testService.Get(), testAdapter.Get(), scheme),
		},
		ExpectUpdates: []runtime.Object{
			testService.
				UserContainer(func(uc *corev1.Container) {
					uc.Image = testImage
				}).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			testAdapter.
				StatusConditions(
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionBuildReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionTargetFound,
						Status: corev1.ConditionTrue,
					},
				).
				StatusLatestImage(testImage).
				Get(),
		},
	}, {
		Name: "adapt application to service",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testAdapter.
				ApplicationRef(testApplication.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testApplication.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
			testService.Get(),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testApplication.Get(), testAdapter.Get(), scheme),
			rtesting.NewTrackRequest(testService.Get(), testAdapter.Get(), scheme),
		},
		ExpectUpdates: []runtime.Object{
			testService.
				UserContainer(func(uc *corev1.Container) {
					uc.Image = testImage
				}).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			testAdapter.
				StatusConditions(
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionBuildReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionTargetFound,
						Status: corev1.ConditionTrue,
					},
				).
				StatusLatestImage(testImage).
				Get(),
		},
	}, {
		Name: "adapt application to service, application not ready",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testAdapter.
				ApplicationRef(testApplication.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testApplication.Get(),
			testService.Get(),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testApplication.Get(), testAdapter.Get(), scheme),
		},
	}, {
		Name: "adapt application to service, application not found",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testAdapter.
				ApplicationRef(testApplication.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testService.Get(),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testApplication.Get(), testAdapter.Get(), scheme),
		},
		ExpectStatusUpdates: []runtime.Object{
			testAdapter.
				StatusConditions(
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionBuildReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionTargetFound,
						Status: corev1.ConditionUnknown,
					},
				).
				Get(),
		},
	}, {
		Name: "adapt application to service, application get failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "Application"),
		},
		GivenObjects: []runtime.Object{
			testAdapter.
				ApplicationRef(testApplication.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testApplication.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
			testService.Get(),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testApplication.Get(), testAdapter.Get(), scheme),
		},
	}, {
		Name: "adapt function to service",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testAdapter.
				FunctionRef(testFunction.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testFunction.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
			testService.Get(),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testFunction.Get(), testAdapter.Get(), scheme),
			rtesting.NewTrackRequest(testService.Get(), testAdapter.Get(), scheme),
		},
		ExpectUpdates: []runtime.Object{
			testService.
				UserContainer(func(uc *corev1.Container) {
					uc.Image = testImage
				}).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			testAdapter.
				StatusConditions(
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionBuildReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionTargetFound,
						Status: corev1.ConditionTrue,
					},
				).
				StatusLatestImage(testImage).
				Get(),
		},
	}, {
		Name: "adapt function to service, function not ready",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testAdapter.
				FunctionRef(testFunction.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testFunction.Get(),
			testService.Get(),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testFunction.Get(), testAdapter.Get(), scheme),
		},
	}, {
		Name: "adapt function to service, function not found",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testAdapter.
				FunctionRef(testFunction.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testService.Get(),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testFunction.Get(), testAdapter.Get(), scheme),
		},
		ExpectStatusUpdates: []runtime.Object{
			testAdapter.
				StatusConditions(
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionBuildReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionTargetFound,
						Status: corev1.ConditionUnknown,
					},
				).
				Get(),
		},
	}, {
		Name: "adapt function to service, get function failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "function"),
		},
		GivenObjects: []runtime.Object{
			testAdapter.
				FunctionRef(testFunction.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testFunction.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
			testService.Get(),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testFunction.Get(), testAdapter.Get(), scheme),
		},
	}, {
		Name: "adapt container to service",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testAdapter.
				ContainerRef(testContainer.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testContainer.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
			testService.Get(),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testContainer.Get(), testAdapter.Get(), scheme),
			rtesting.NewTrackRequest(testService.Get(), testAdapter.Get(), scheme),
		},
		ExpectUpdates: []runtime.Object{
			testService.
				UserContainer(func(uc *corev1.Container) {
					uc.Image = testImage
				}).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			testAdapter.
				StatusConditions(
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionBuildReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionTargetFound,
						Status: corev1.ConditionTrue,
					},
				).
				StatusLatestImage(testImage).
				Get(),
		},
	}, {
		Name: "adapt container to service, container not ready",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testAdapter.
				ContainerRef(testContainer.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testContainer.Get(),
			testService.Get(),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testContainer.Get(), testAdapter.Get(), scheme),
		},
	}, {
		Name: "adapt container to service, container not found",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testAdapter.
				ContainerRef(testContainer.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testService.Get(),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testContainer.Get(), testAdapter.Get(), scheme),
		},
		ExpectStatusUpdates: []runtime.Object{
			testAdapter.
				StatusConditions(
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionBuildReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionTargetFound,
						Status: corev1.ConditionUnknown,
					},
				).
				Get(),
		},
	}, {
		Name: "adapt container to service, get container failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "Container"),
		},
		GivenObjects: []runtime.Object{
			testAdapter.
				ContainerRef(testContainer.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testContainer.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
			testService.Get(),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testContainer.Get(), testAdapter.Get(), scheme),
		},
	}, {
		Name: "adapt container to service, service not found",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testAdapter.
				ContainerRef(testContainer.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testContainer.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testContainer.Get(), testAdapter.Get(), scheme),
			rtesting.NewTrackRequest(testService.Get(), testAdapter.Get(), scheme),
		},
		ExpectStatusUpdates: []runtime.Object{
			testAdapter.
				StatusConditions(
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionBuildReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:    knativev1alpha1.AdapterConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  "NotFound",
						Message: `The service "my-service" was not found.`,
					},
					apis.Condition{
						Type:    knativev1alpha1.AdapterConditionTargetFound,
						Status:  corev1.ConditionFalse,
						Reason:  "NotFound",
						Message: `The service "my-service" was not found.`,
					},
				).
				StatusLatestImage(testImage).
				Get(),
		},
	}, {
		Name: "adapt container to service, get service failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "Service"),
		},
		GivenObjects: []runtime.Object{
			testAdapter.
				ContainerRef(testContainer.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testContainer.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
			testService.Get(),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testContainer.Get(), testAdapter.Get(), scheme),
			rtesting.NewTrackRequest(testService.Get(), testAdapter.Get(), scheme),
		},
	}, {
		Name: "adapt container to service, service is up to date",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testAdapter.
				ContainerRef(testContainer.Get().Name).
				ServiceRef(testService.Get().Name).
				StatusConditions(
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionBuildReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionTargetFound,
						Status: corev1.ConditionTrue,
					},
				).
				StatusLatestImage(testImage).
				Get(),
			testContainer.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
			testService.
				UserContainer(func(uc *corev1.Container) {
					uc.Image = testImage
				}).
				Get(),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testContainer.Get(), testAdapter.Get(), scheme),
			rtesting.NewTrackRequest(testService.Get(), testAdapter.Get(), scheme),
		},
	}, {
		Name: "adapt container to service, update service failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("update", "Service"),
		},
		GivenObjects: []runtime.Object{
			testAdapter.
				ContainerRef(testContainer.Get().Name).
				ServiceRef(testService.Get().Name).
				Get(),
			testContainer.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
			testService.Get(),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testContainer.Get(), testAdapter.Get(), scheme),
			rtesting.NewTrackRequest(testService.Get(), testAdapter.Get(), scheme),
		},
		ExpectUpdates: []runtime.Object{
			testService.
				UserContainer(func(uc *corev1.Container) {
					uc.Image = testImage
				}).
				Get(),
		},
	}, {
		Name: "adapt container to configuration",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testAdapter.
				ContainerRef(testContainer.Get().Name).
				ConfigurationRef(testConfiguration.Get().Name).
				Get(),
			testContainer.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
			testConfiguration.Get(),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testContainer.Get(), testAdapter.Get(), scheme),
			rtesting.NewTrackRequest(testConfiguration.Get(), testAdapter.Get(), scheme),
		},
		ExpectUpdates: []runtime.Object{
			testConfiguration.
				UserContainer(func(uc *corev1.Container) {
					uc.Image = testImage
				}).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			testAdapter.
				StatusConditions(
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionBuildReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionTargetFound,
						Status: corev1.ConditionTrue,
					},
				).
				StatusLatestImage(testImage).
				Get(),
		},
	}, {
		Name: "adapt container to configuration, configuration not found",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testAdapter.
				ContainerRef(testContainer.Get().Name).
				ConfigurationRef(testConfiguration.Get().Name).
				Get(),
			testContainer.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testContainer.Get(), testAdapter.Get(), scheme),
			rtesting.NewTrackRequest(testConfiguration.Get(), testAdapter.Get(), scheme),
		},
		ExpectStatusUpdates: []runtime.Object{
			testAdapter.
				StatusConditions(
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionBuildReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:    knativev1alpha1.AdapterConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  "NotFound",
						Message: `The configuration "my-configuration" was not found.`,
					},
					apis.Condition{
						Type:    knativev1alpha1.AdapterConditionTargetFound,
						Status:  corev1.ConditionFalse,
						Reason:  "NotFound",
						Message: `The configuration "my-configuration" was not found.`,
					},
				).
				StatusLatestImage(testImage).
				Get(),
		},
	}, {
		Name: "adapt container to configuration, get configuration failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "Configuration"),
		},
		GivenObjects: []runtime.Object{
			testAdapter.
				ContainerRef(testContainer.Get().Name).
				ConfigurationRef(testConfiguration.Get().Name).
				Get(),
			testContainer.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
			testConfiguration.Get(),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testContainer.Get(), testAdapter.Get(), scheme),
			rtesting.NewTrackRequest(testConfiguration.Get(), testAdapter.Get(), scheme),
		},
	}, {
		Name: "adapt container to configuration, configuration is up to date",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testAdapter.
				ContainerRef(testContainer.Get().Name).
				ConfigurationRef(testConfiguration.Get().Name).
				StatusConditions(
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionBuildReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   knativev1alpha1.AdapterConditionTargetFound,
						Status: corev1.ConditionTrue,
					},
				).
				StatusLatestImage(testImage).
				Get(),
			testContainer.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
			testConfiguration.
				UserContainer(func(uc *corev1.Container) {
					uc.Image = testImage
				}).
				Get(),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testContainer.Get(), testAdapter.Get(), scheme),
			rtesting.NewTrackRequest(testConfiguration.Get(), testAdapter.Get(), scheme),
		},
	}, {
		Name: "adapt container to configuration, update configuration failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("update", "Configuration"),
		},
		GivenObjects: []runtime.Object{
			testAdapter.
				ContainerRef(testContainer.Get().Name).
				ConfigurationRef(testConfiguration.Get().Name).
				Get(),
			testContainer.
				StatusLatestImage(testImage).
				StatusReady().
				Get(),
			testConfiguration.Get(),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testContainer.Get(), testAdapter.Get(), scheme),
			rtesting.NewTrackRequest(testConfiguration.Get(), testAdapter.Get(), scheme),
		},
		ExpectUpdates: []runtime.Object{
			testConfiguration.
				UserContainer(func(uc *corev1.Container) {
					uc.Image = testImage
				}).
				Get(),
		},
	}}

	table.Test(t, scheme, func(t *testing.T, row *rtesting.Testcase, client client.Client, tracker tracker.Tracker, log logr.Logger) reconcile.Reconciler {
		return &knative.AdapterReconciler{
			Client:  client,
			Log:     log,
			Scheme:  scheme,
			Tracker: tracker,
		}
	})
}
