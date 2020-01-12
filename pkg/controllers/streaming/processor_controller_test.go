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

package streaming

import (
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
	kedav1alpha1 "github.com/projectriff/system/pkg/apis/thirdparty/keda/v1alpha1"
	rtesting "github.com/projectriff/system/pkg/controllers/testing"
	"github.com/projectriff/system/pkg/controllers/testing/factories"
	"github.com/projectriff/system/pkg/tracker"
)

func TestReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = streamingv1alpha1.AddToScheme(scheme)
	_ = kedav1alpha1.AddToScheme(scheme)
	_ = buildv1alpha1.AddToScheme(scheme)

	const (
		testSha256         = "faa5faa5faa5faa5faa5faa5faa5faa5faa5faa5faa5faa5faa5faa5faa5faa5"
		testNamespace      = "test-namespace"
		testName           = "test-processor"
		testProcessorImage = "test-processor-image@sha256:" + testSha256
		testDefaultImage   = "test-default-image@sha256:" + testSha256
		testFunction       = "test-function"
		testFunctionImage  = "test-function-image@sha256:" + testSha256
		testContainer      = "test-container"
		testContainerImage = "test-container-image@sha256:" + testSha256
	)

	processorGiven := factories.Processor().
		NamespaceName(testNamespace, testName).
		PodTemplateSpec(func(spec factories.PodTemplateSpec) {
			spec.ContainerNamed(testContainer, func(c *corev1.Container) {
				c.Image = testDefaultImage
			})
		})
	imageNamesConfigMapGiven := factories.ConfigMap().
		NamespaceName(testNamespace, processorImages).
		AddData(processorImageKey, testProcessorImage)
	containerGiven := factories.Container().
		NamespaceName(testNamespace, testContainer).
		StatusLatestImage(testContainerImage)
	functionGiven := factories.Function().
		NamespaceName(testNamespace, testFunction)
	deploymentCreate := factories.Deployment().
		ObjectMeta(func(om factories.ObjectMeta) {
			om.Namespace(testNamespace)
			om.GenerateName("%s-processor-", testName)
			om.AddLabel("streaming.projectriff.io/processor", testName)
			om.ControlledBy(processorGiven, scheme)
		}).
		Replicas(1).
		AddSelectorLabel("streaming.projectriff.io/processor", testName).
		PodTemplateSpec(func(pts factories.PodTemplateSpec) {
			pts.AddLabel("streaming.projectriff.io/processor", testName)
		})
	scaledObjectCreate := factories.KedaScaledObject().
		ObjectMeta(func(om factories.ObjectMeta) {
			om.Namespace(testNamespace)
			om.GenerateName("%s-processor-", testName)
			om.AddLabel("streaming.projectriff.io/processor", testName)
			om.AddLabel("deploymentName", testName+"-processor-001") // TODO: this label looks bogus
			om.ControlledBy(processorGiven, scheme)
		}).
		ScaleTargetRefDeployment("%s-processor-001", testName).
		PollingInterval(1).
		CooldownPeriod(30).
		MinReplicaCount(1).
		MaxReplicaCount(30)

	testCoreContainer := func(imageRef string) func(container *corev1.Container) {
		return func(container *corev1.Container) {
			container.Name = testContainer
			container.Image = imageRef
			container.Ports = []corev1.ContainerPort{{ContainerPort: 8081}}
		}
	}

	processorCoreContainer := func(container *corev1.Container) {
		container.Name = "processor"
		container.Image = testProcessorImage
		container.Env = []corev1.EnvVar{
			{Name: "CNB_BINDINGS", Value: "/var/riff/bindings"},
			{Name: "INPUT_START_OFFSETS"},
			{Name: "INPUT_NAMES"},
			{Name: "OUTPUT_NAMES"},
			{Name: "GROUP", Value: testName},
			{Name: "FUNCTION", Value: "localhost:8081"},
		}
		container.ImagePullPolicy = "IfNotPresent"
	}

	processorConditionDeploymentReady := factories.Condition().Type(streamingv1alpha1.ProcessorConditionDeploymentReady)
	processorConditionReady := factories.Condition().Type(streamingv1alpha1.ProcessorConditionReady)
	processorConditionScaledObjectReady := factories.Condition().Type(streamingv1alpha1.ProcessorConditionScaledObjectReady)
	processorConditionStreamsReady := factories.Condition().Type(streamingv1alpha1.ProcessorConditionStreamsReady)

	table := rtesting.Table{{
		Name:         "processor does not exist",
		Key:          types.NamespacedName{Namespace: testNamespace, Name: testName},
		ExpectTracks: []rtesting.TrackRequest{},
	}, {
		Name: "getting processor fails",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "Processor"),
		},
		ExpectTracks: []rtesting.TrackRequest{},
		ShouldErr:    true,
	}, {
		Name: "configMap does not exist",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []rtesting.Factory{
			processorGiven,
		},
		ShouldErr: true,
		Verify:    rtesting.AssertErrorMessagef("configmaps %q not found", processorImages),
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(imageNamesConfigMapGiven, processorGiven, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			processorGiven.
				StatusConditions(
					processorConditionDeploymentReady.Unknown(),
					processorConditionReady.Unknown(),
					processorConditionScaledObjectReady.Unknown(),
					processorConditionStreamsReady.Unknown(),
				),
		},
	}, {
		Name: "processor is marked for deletion",
		GivenObjects: []rtesting.Factory{
			processorGiven.
				ObjectMeta(func(meta factories.ObjectMeta) {
					meta.Deleted(1)
				}),
		},
		Key: types.NamespacedName{Namespace: testNamespace, Name: testName},
	}, {
		Name: "getting configMap fails",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []rtesting.Factory{
			processorGiven,
		},
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "ConfigMap"),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(imageNamesConfigMapGiven, processorGiven, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			processorGiven.
				StatusConditions(
					processorConditionDeploymentReady.Unknown(),
					processorConditionReady.Unknown(),
					processorConditionScaledObjectReady.Unknown(),
					processorConditionStreamsReady.Unknown(),
				),
		},
	}, {
		Name: "processor sidecar image not present in configMap",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []rtesting.Factory{
			processorGiven,
			factories.ConfigMap().
				NamespaceName(testNamespace, processorImages),
		},
		ShouldErr: true,
		Verify:    rtesting.AssertErrorMessagef("missing processor image configuration"),
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(imageNamesConfigMapGiven, processorGiven, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			processorGiven.
				StatusConditions(
					processorConditionDeploymentReady.Unknown(),
					processorConditionReady.Unknown(),
					processorConditionScaledObjectReady.Unknown(),
					processorConditionStreamsReady.Unknown(),
				).
				StatusLatestImage(testDefaultImage),
		},
	}, {
		Name: "default application image not set",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []rtesting.Factory{
			factories.Processor().
				NamespaceName(testNamespace, testName),
			imageNamesConfigMapGiven,
		},
		ShouldErr: true,
		Verify:    rtesting.AssertErrorMessagef("could not resolve an image"),
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(imageNamesConfigMapGiven, processorGiven, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			processorGiven.
				StatusConditions(
					processorConditionDeploymentReady.Unknown(),
					processorConditionReady.Unknown(),
					processorConditionScaledObjectReady.Unknown(),
					processorConditionStreamsReady.Unknown(),
				),
		},
	}, {
		Name: "successful reconciliation",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []rtesting.Factory{
			processorGiven,
			imageNamesConfigMapGiven,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(imageNamesConfigMapGiven, processorGiven, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "Created",
				`Created Deployment "%s-processor-001"`, testName),
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "Created",
				`Created ScaledObject "%s-processor-002"`, testName),
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectCreates: []rtesting.Factory{
			deploymentCreate.
				PodTemplateSpec(func(pts factories.PodTemplateSpec) {
					pts.ContainerNamed(testContainer, testCoreContainer(testDefaultImage))
					pts.ContainerNamed("processor", processorCoreContainer)
				}),
			scaledObjectCreate,
		},
		ExpectStatusUpdates: []rtesting.Factory{
			processorGiven.
				StatusConditions(
					processorConditionDeploymentReady.Unknown(),
					processorConditionReady.Unknown(),
					processorConditionScaledObjectReady.True(),
					processorConditionStreamsReady.True(),
				).
				StatusLatestImage(testDefaultImage).
				StatusDeploymentRef(testName + "-processor-001").
				StatusScaledObjectRef(testName + "-processor-002"),
		},
	}, {
		Name: "deployment creation fails",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []rtesting.Factory{
			processorGiven,
			imageNamesConfigMapGiven,
		},
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("create", "Deployment"),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(imageNamesConfigMapGiven, processorGiven, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeWarning, "CreationFailed",
				`Failed to create Deployment "": inducing failure for create Deployment`),
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectCreates: []rtesting.Factory{
			deploymentCreate.
				PodTemplateSpec(func(pts factories.PodTemplateSpec) {
					pts.ContainerNamed(testContainer, testCoreContainer(testDefaultImage))
					pts.ContainerNamed("processor", processorCoreContainer)
				}),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			processorGiven.
				StatusConditions(
					processorConditionDeploymentReady.Unknown(),
					processorConditionReady.Unknown(),
					processorConditionScaledObjectReady.Unknown(),
					processorConditionStreamsReady.Unknown(),
				).
				StatusLatestImage(testDefaultImage),
		},
	}, {
		Name: "scaled object creation fails",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []rtesting.Factory{
			processorGiven,
			imageNamesConfigMapGiven,
		},
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("create", "ScaledObject"),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(imageNamesConfigMapGiven, processorGiven, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "Created",
				`Created Deployment "%s-processor-001"`, testName),
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeWarning, "CreationFailed",
				`Failed to create ScaledObject "": inducing failure for create ScaledObject`),
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectCreates: []rtesting.Factory{
			deploymentCreate.
				PodTemplateSpec(func(pts factories.PodTemplateSpec) {
					pts.ContainerNamed(testContainer, testCoreContainer(testDefaultImage))
					pts.ContainerNamed("processor", processorCoreContainer)
				}),
			scaledObjectCreate,
		},
		ExpectStatusUpdates: []rtesting.Factory{
			processorGiven.
				StatusConditions(
					processorConditionDeploymentReady.Unknown(),
					processorConditionReady.Unknown(),
					processorConditionScaledObjectReady.Unknown(),
					processorConditionStreamsReady.True(),
				).
				StatusLatestImage(testDefaultImage).
				StatusDeploymentRef(testName + "-processor-001"),
		},
	}, {
		Name: "successful reconciliation with unsatisfied function reference",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []rtesting.Factory{
			processorGiven.
				SpecBuildFunctionRef(testFunction),
			imageNamesConfigMapGiven,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(imageNamesConfigMapGiven, processorGiven, scheme),
			rtesting.NewTrackRequest(functionGiven, processorGiven, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			processorGiven.
				StatusConditions(
					processorConditionDeploymentReady.Unknown(),
					processorConditionReady.Unknown(),
					processorConditionScaledObjectReady.Unknown(),
					processorConditionStreamsReady.Unknown(),
				),
		},
	}, {
		Name: "get function fails",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []rtesting.Factory{
			processorGiven.
				SpecBuildFunctionRef(testFunction),
			imageNamesConfigMapGiven,
			functionGiven,
		},
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "Function"),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(imageNamesConfigMapGiven, processorGiven, scheme),
			rtesting.NewTrackRequest(functionGiven, processorGiven, scheme),
		},
	}, {
		Name: "successful reconciliation with satisfied function reference",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []rtesting.Factory{
			processorGiven.
				SpecBuildFunctionRef(testFunction),
			imageNamesConfigMapGiven,
			functionGiven.
				StatusLatestImage(testFunctionImage),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(imageNamesConfigMapGiven, processorGiven, scheme),
			rtesting.NewTrackRequest(functionGiven, processorGiven, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "Created",
				`Created Deployment "%s-processor-001"`, testName),
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "Created",
				`Created ScaledObject "%s-processor-002"`, testName),
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectCreates: []rtesting.Factory{
			deploymentCreate.
				PodTemplateSpec(func(pts factories.PodTemplateSpec) {
					pts.ContainerNamed(testContainer, testCoreContainer(testFunctionImage))
					pts.ContainerNamed("processor", processorCoreContainer)
				}),
			scaledObjectCreate,
		},
		ExpectStatusUpdates: []rtesting.Factory{
			processorGiven.
				StatusConditions(
					processorConditionDeploymentReady.Unknown(),
					processorConditionReady.Unknown(),
					processorConditionScaledObjectReady.True(),
					processorConditionStreamsReady.True(),
				).
				StatusLatestImage(testFunctionImage).
				StatusDeploymentRef(testName + "-processor-001").
				StatusScaledObjectRef(testName + "-processor-002"),
		},
	}, {
		Name: "successful reconciliation with unsatisfied container reference",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []rtesting.Factory{
			processorGiven.
				SpecBuildContainerRef(testContainer),
			imageNamesConfigMapGiven,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(imageNamesConfigMapGiven, processorGiven, scheme),
			rtesting.NewTrackRequest(containerGiven, processorGiven, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			processorGiven.
				StatusConditions(
					processorConditionDeploymentReady.Unknown(),
					processorConditionReady.Unknown(),
					processorConditionScaledObjectReady.Unknown(),
					processorConditionStreamsReady.Unknown(),
				),
		},
	}, {
		Name: "get container fails",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []rtesting.Factory{
			processorGiven.
				SpecBuildContainerRef(testContainer),
			imageNamesConfigMapGiven,
		},
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "Container"),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(imageNamesConfigMapGiven, processorGiven, scheme),
			rtesting.NewTrackRequest(containerGiven, processorGiven, scheme),
		},
	}, {
		Name: "successful reconciliation with satisfied container reference",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []rtesting.Factory{
			processorGiven.
				SpecBuildContainerRef(testContainer),
			imageNamesConfigMapGiven,
			containerGiven,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(imageNamesConfigMapGiven, processorGiven, scheme),
			rtesting.NewTrackRequest(containerGiven, processorGiven, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "Created",
				`Created Deployment "%s-processor-001"`, testName),
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "Created",
				`Created ScaledObject "%s-processor-002"`, testName),
			rtesting.NewEvent(processorGiven, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectCreates: []rtesting.Factory{
			deploymentCreate.
				PodTemplateSpec(func(pts factories.PodTemplateSpec) {
					pts.ContainerNamed(testContainer, testCoreContainer(testContainerImage))
					pts.ContainerNamed("processor", processorCoreContainer)
				}),
			scaledObjectCreate,
		},
		ExpectStatusUpdates: []rtesting.Factory{
			processorGiven.
				StatusConditions(
					processorConditionDeploymentReady.Unknown(),
					processorConditionReady.Unknown(),
					processorConditionScaledObjectReady.True(),
					processorConditionStreamsReady.True(),
				).
				StatusLatestImage(testContainerImage).
				StatusDeploymentRef(testName + "-processor-001").
				StatusScaledObjectRef(testName + "-processor-002"),
		},
	}}

	table.Test(t, scheme, func(t *testing.T, row *rtesting.Testcase, client client.Client, tracker tracker.Tracker, recorder record.EventRecorder, log logr.Logger) reconcile.Reconciler {
		return &ProcessorReconciler{
			Client:    client,
			Recorder:  recorder,
			Log:       log,
			Scheme:    scheme,
			Tracker:   tracker,
			Namespace: testNamespace,
		}
	})
}
