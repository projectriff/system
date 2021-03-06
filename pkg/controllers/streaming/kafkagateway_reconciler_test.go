/*
Copyright 2020 the original author or authors.

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

package streaming_test

import (
	"fmt"
	"testing"

	"github.com/vmware-labs/reconciler-runtime/reconcilers"
	rtesting "github.com/vmware-labs/reconciler-runtime/testing"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
	"github.com/projectriff/system/pkg/controllers/streaming"
	"github.com/projectriff/system/pkg/controllers/testing/factories"
)

func TestKafkaGatewayReconciler(t *testing.T) {
	testNamespace := "test-namespace"
	testSystemNamespace := "system-namespace"
	testName := "test-gateway"
	testKey := types.NamespacedName{Namespace: testNamespace, Name: testName}
	testImagePrefix := "example.com/repo"
	testGatewayImage := fmt.Sprintf("%s/%s", testImagePrefix, "gateway")
	testProvisionerImage := fmt.Sprintf("%s/%s", testImagePrefix, "provisioner")
	testProvisionerHostname := fmt.Sprintf("%s.%s.svc.cluster.local", testName, testNamespace)
	testProvisionerURL := fmt.Sprintf("http://%s", testProvisionerHostname)
	testBootstrapServers := "kafka.local:9092"

	kafkaGatewayImages := "riff-streaming-kafka-gateway" // contains image names for the kafka gateway
	gatewayImageKey := "gatewayImage"
	provisionerImageKey := "provisionerImage"

	kafkaGatewayConditionGatewayReady := factories.Condition().Type(streamingv1alpha1.KafkaGatewayConditionGatewayReady)
	kafkaGatewayConditionReady := factories.Condition().Type(streamingv1alpha1.KafkaGatewayConditionReady)
	gatewayConditionReady := factories.Condition().Type(streamingv1alpha1.GatewayConditionReady)

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = streamingv1alpha1.AddToScheme(scheme)

	kafkaGatewayImagesConfigMap := factories.ConfigMap().
		NamespaceName(testSystemNamespace, kafkaGatewayImages).
		AddData(gatewayImageKey, testGatewayImage).
		AddData(provisionerImageKey, testProvisionerImage)

	kafkaGatewayMinimal := factories.KafkaGateway().
		NamespaceName(testNamespace, testName).
		ObjectMeta(func(om factories.ObjectMeta) {
			om.Created(1)
			om.Generation(1)
		}).
		BootstrapServers(testBootstrapServers)
	kafkaGateway := kafkaGatewayMinimal.
		StatusGatewayRef(testName).
		StatusGatewayImage(testGatewayImage).
		StatusProvisionerImage(testProvisionerImage)
	kafkaGatewayReady := kafkaGateway.
		StatusObservedGeneration(1).
		StatusConditions(
			kafkaGatewayConditionGatewayReady.True(),
			kafkaGatewayConditionReady.True(),
		)

	gatewayCreate := factories.Gateway().
		NamespaceName(testNamespace, testName).
		ObjectMeta(func(om factories.ObjectMeta) {
			om.AddLabel(streamingv1alpha1.KafkaGatewayLabelKey, testName)
			om.AddLabel(streamingv1alpha1.GatewayTypeLabelKey, streamingv1alpha1.KafkaGatewayType)
			om.ControlledBy(kafkaGateway, scheme)
		}).
		Ports(
			corev1.ServicePort{Name: "gateway", Port: 6565},
			corev1.ServicePort{Name: "provisioner", Port: 80, TargetPort: intstr.FromInt(8080)},
		)
	gatewayGiven := gatewayCreate.
		ObjectMeta(func(om factories.ObjectMeta) {
			om.Created(1)
		}).
		StatusAddress(testProvisionerURL)
	gatewayComplete := gatewayGiven.
		PodTemplateSpec(func(pts factories.PodTemplateSpec) {
			pts.AddLabel(streamingv1alpha1.KafkaGatewayLabelKey, testName)
			pts.AddLabel(streamingv1alpha1.GatewayTypeLabelKey, streamingv1alpha1.KafkaGatewayType)
			pts.ContainerNamed("gateway", func(c *corev1.Container) {
				c.Image = testGatewayImage
				c.Env = []corev1.EnvVar{
					{Name: "kafka_bootstrapServers", Value: testBootstrapServers},
					{Name: "storage_positions_type", Value: "MEMORY"},
					{Name: "storage_records_type", Value: "KAFKA"},
					{Name: "server_port", Value: "8000"},
				}
			})
			pts.ContainerNamed("provisioner", func(c *corev1.Container) {
				c.Image = testProvisionerImage
				c.Env = []corev1.EnvVar{
					{Name: "GATEWAY", Value: fmt.Sprintf("%s:6565", testProvisionerHostname)},
					{Name: "BROKER", Value: testBootstrapServers},
				}
			})
		})

	rts := rtesting.ReconcilerTestSuite{{
		Name: "kafkagateway does not exist",
		Key:  testKey,
	}, {
		Name: "ignore deleted kafkagateway",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			kafkaGateway.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.Deleted(1)
				}),
		},
	}, {
		Name: "error fetching kafkagateway",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "KafkaGateway"),
		},
		GivenObjects: []rtesting.Factory{
			kafkaGateway,
		},
		ShouldErr: true,
	}, {
		Name: "creates gateway",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			kafkaGatewayMinimal,
			kafkaGatewayImagesConfigMap,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(kafkaGatewayImagesConfigMap, kafkaGateway, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(kafkaGateway, scheme, corev1.EventTypeNormal, "Created",
				`Created Gateway "%s"`, testName),
			rtesting.NewEvent(kafkaGateway, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectCreates: []rtesting.Factory{
			gatewayCreate,
		},
		ExpectStatusUpdates: []rtesting.Factory{
			kafkaGateway.
				StatusObservedGeneration(1).
				StatusConditions(
					kafkaGatewayConditionGatewayReady.Unknown(),
					kafkaGatewayConditionReady.Unknown(),
				),
		},
	}, {
		Name: "propagate address",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			kafkaGateway,
			kafkaGatewayImagesConfigMap,
			gatewayGiven,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(kafkaGatewayImagesConfigMap, kafkaGateway, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(kafkaGateway, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			kafkaGateway.
				StatusAddress(testProvisionerURL).
				StatusObservedGeneration(1).
				StatusConditions(
					kafkaGatewayConditionGatewayReady.Unknown(),
					kafkaGatewayConditionReady.Unknown(),
				),
		},
	}, {
		Name: "updates gateway",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			kafkaGateway.
				StatusAddress(testProvisionerURL),
			kafkaGatewayImagesConfigMap,
			gatewayGiven,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(kafkaGatewayImagesConfigMap, kafkaGateway, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(kafkaGateway, scheme, corev1.EventTypeNormal, "Updated",
				`Updated Gateway "%s"`, testName),
			rtesting.NewEvent(kafkaGateway, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectUpdates: []rtesting.Factory{
			gatewayComplete,
		},
		ExpectStatusUpdates: []rtesting.Factory{
			kafkaGateway.
				StatusAddress(testProvisionerURL).
				StatusObservedGeneration(1).
				StatusConditions(
					kafkaGatewayConditionGatewayReady.Unknown(),
					kafkaGatewayConditionReady.Unknown(),
				),
		},
	}, {
		Name: "ready",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			kafkaGateway.
				StatusAddress(testProvisionerURL),
			kafkaGatewayImagesConfigMap,
			gatewayComplete.
				StatusReady(),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(kafkaGatewayImagesConfigMap, kafkaGateway, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(kafkaGateway, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			kafkaGateway.
				StatusAddress(testProvisionerURL).
				StatusObservedGeneration(1).
				StatusConditions(
					kafkaGatewayConditionGatewayReady.True(),
					kafkaGatewayConditionReady.True(),
				),
		},
	}, {
		Name: "not ready",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			kafkaGateway.
				StatusAddress(testProvisionerURL),
			kafkaGatewayImagesConfigMap,
			gatewayComplete.
				StatusConditions(
					gatewayConditionReady.False().Reason("TestReason", "a human readable message"),
				),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(kafkaGatewayImagesConfigMap, kafkaGateway, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(kafkaGateway, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			kafkaGateway.
				StatusAddress(testProvisionerURL).
				StatusObservedGeneration(1).
				StatusConditions(
					kafkaGatewayConditionGatewayReady.False().Reason("TestReason", "a human readable message"),
					kafkaGatewayConditionReady.False().Reason("TestReason", "a human readable message"),
				),
		},
	}, {
		Name: "missing gateway images configmap",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			kafkaGatewayReady,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(kafkaGatewayImagesConfigMap, kafkaGateway, scheme),
		},
	}, {
		Name: "invalid address",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			kafkaGatewayReady.
				StatusAddress("\000"),
			kafkaGatewayImagesConfigMap,
			gatewayGiven.
				StatusAddress("\000"),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(kafkaGatewayImagesConfigMap, kafkaGateway, scheme),
		},
	}, {
		Name: "conflicting gateway",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("create", "Gateway", rtesting.InduceFailureOpts{
				Error: apierrs.NewAlreadyExists(schema.GroupResource{}, testName),
			}),
		},
		GivenObjects: []rtesting.Factory{
			kafkaGatewayMinimal,
			kafkaGatewayImagesConfigMap,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(kafkaGatewayImagesConfigMap, kafkaGateway, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(kafkaGateway, scheme, corev1.EventTypeWarning, "CreationFailed",
				`Failed to create Gateway "%s":  "%s" already exists`, testName, testName),
			rtesting.NewEvent(kafkaGateway, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectCreates: []rtesting.Factory{
			gatewayCreate,
		},
		ExpectStatusUpdates: []rtesting.Factory{
			kafkaGatewayMinimal.
				StatusObservedGeneration(1).
				StatusConditions(
					kafkaGatewayConditionGatewayReady.False().Reason("NotOwned", `There is an existing Gateway "test-gateway" that the KafkaGateway does not own.`),
					kafkaGatewayConditionReady.False().Reason("NotOwned", `There is an existing Gateway "test-gateway" that the KafkaGateway does not own.`),
				).
				StatusGatewayImage(testGatewayImage).
				StatusProvisionerImage(testProvisionerImage),
		},
	}, {
		Name: "conflicting gateway, owned",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("create", "Gateway", rtesting.InduceFailureOpts{
				Error: apierrs.NewAlreadyExists(schema.GroupResource{}, testName),
			}),
		},
		GivenObjects: []rtesting.Factory{
			kafkaGatewayMinimal,
			kafkaGatewayImagesConfigMap,
		},
		APIGivenObjects: []rtesting.Factory{
			gatewayGiven,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(kafkaGatewayImagesConfigMap, kafkaGateway, scheme),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(kafkaGateway, scheme, corev1.EventTypeWarning, "CreationFailed",
				`Failed to create Gateway "%s":  "%s" already exists`, testName, testName),
			rtesting.NewEvent(kafkaGateway, scheme, corev1.EventTypeNormal, "StatusUpdated",
				`Updated status`),
		},
		ExpectCreates: []rtesting.Factory{
			gatewayCreate,
		},
		ExpectStatusUpdates: []rtesting.Factory{
			kafkaGatewayMinimal.
				StatusConditions(
					kafkaGatewayConditionGatewayReady.Unknown(),
					kafkaGatewayConditionReady.Unknown(),
				).
				StatusGatewayImage(testGatewayImage).
				StatusProvisionerImage(testProvisionerImage),
		},
	}}

	rts.Test(t, scheme, func(t *testing.T, rtc *rtesting.ReconcilerTestCase, c reconcilers.Config) reconcile.Reconciler {
		return streaming.KafkaGatewayReconciler(c, testSystemNamespace)
	})
}
