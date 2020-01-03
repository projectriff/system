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

package core_test

import (
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/projectriff/system/pkg/apis"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	corev1alpha1 "github.com/projectriff/system/pkg/apis/core/v1alpha1"
	"github.com/projectriff/system/pkg/controllers/core"
	rtesting "github.com/projectriff/system/pkg/controllers/testing"
	"github.com/projectriff/system/pkg/controllers/testing/factories"
	"github.com/projectriff/system/pkg/tracker"
)

func TestDeployerReconcile(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-deployer"
	testKey := types.NamespacedName{Namespace: testNamespace, Name: testName}
	testImagePrefix := "example.com/repo"
	testSha256 := "cf8b4c69d5460f88530e1c80b8856a70801f31c50b191c8413043ba9b160a43e"
	testImage := fmt.Sprintf("%s@sha256:%s", testImagePrefix, testSha256)
	testConditionReason := "TestReason"
	testConditionMessage := "meaningful, yet concise"
	testDomain := "example.com"
	testHost := fmt.Sprintf("%s.%s.%s", testName, testNamespace, testDomain)
	testURL := fmt.Sprintf("http://%s", testHost)
	testAddressURL := fmt.Sprintf("http://%s.%s.svc.cluster.local", testName, testNamespace)
	testLabelKey := "test-label-key"
	testLabelValue := "test-label-value"

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = buildv1alpha1.AddToScheme(scheme)
	_ = corev1alpha1.AddToScheme(scheme)

	deployerMinimal := factories.DeployerCore().
		NamespaceName(testNamespace, testName).
		Get()
	deployerValid := factories.DeployerCore(deployerMinimal).
		Image(testImage).
		IngressPolicy(corev1alpha1.IngressPolicyClusterLocal).
		Get()

	deploymentCreate := factories.Deployment().
		ObjectMeta(func(om factories.ObjectMeta) {
			om.Namespace(testNamespace)
			om.GenerateName("%s-deployer-", deployerMinimal.Name)
			om.AddLabel(corev1alpha1.DeployerLabelKey, deployerMinimal.Name)
			om.ControlledBy(deployerMinimal, scheme)
		}).
		AddSelectorLabel(corev1alpha1.DeployerLabelKey, deployerMinimal.Name).
		HandlerContainer(func(container *corev1.Container) {
			container.Image = testImage
			container.Ports = []corev1.ContainerPort{
				{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
			}
			container.Env = []corev1.EnvVar{
				{Name: "PORT", Value: "8080"},
			}
			container.LivenessProbe = &corev1.Probe{
				Handler: corev1.Handler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromInt(8080),
					},
				},
			}
			container.ReadinessProbe = &corev1.Probe{
				Handler: corev1.Handler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromInt(8080),
					},
				},
			}
		}).
		Get()
	deploymentGiven := factories.Deployment(deploymentCreate).
		ObjectMeta(func(om factories.ObjectMeta) {
			om.Name("%s%s", om.Get().GenerateName, "000")
			om.Created(1)
		}).
		Get()

	serviceCreate := factories.Service().
		NamespaceName(testNamespace, testName).
		ObjectMeta(func(om factories.ObjectMeta) {
			om.AddLabel(corev1alpha1.DeployerLabelKey, deployerMinimal.Name)
			om.ControlledBy(deployerMinimal, scheme)
		}).
		AddSelectorLabel(corev1alpha1.DeployerLabelKey, deployerMinimal.Name).
		Ports(
			corev1.ServicePort{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			},
		).
		Get()
	serviceGiven := factories.Service(serviceCreate).
		ObjectMeta(func(om factories.ObjectMeta) {
			om.Created(1)
			om.ControlledBy(deployerMinimal, scheme)
		}).
		Get()

	ingressCreate := factories.Ingress().
		ObjectMeta(func(om factories.ObjectMeta) {
			om.Namespace(testNamespace)
			om.GenerateName("%s-deployer-", deployerMinimal.Name)
			om.AddLabel(corev1alpha1.DeployerLabelKey, deployerMinimal.Name)
			om.ControlledBy(deployerMinimal, scheme)
		}).
		HostToService(testHost, serviceGiven.Name).
		Get()
	ingressGiven := factories.Ingress(ingressCreate).
		ObjectMeta(func(om factories.ObjectMeta) {
			om.Name("%s%s", om.Get().GenerateName, "000")
			om.Created(1)
		}).
		Get()

	testApplication := factories.Application().
		NamespaceName(testNamespace, "my-application").
		StatusLatestImage(testImage).
		Get()
	testFunction := factories.Function().
		NamespaceName(testNamespace, "my-function").
		StatusLatestImage(testImage).
		Get()
	testContainer := factories.Container().
		NamespaceName(testNamespace, "my-container").
		StatusLatestImage(testImage).
		Get()

	testSettings := factories.ConfigMap().
		NamespaceName("riff-system", "riff-core-settings").
		AddData("defaultDomain", "example.com").
		Get()

	table := rtesting.Table{{
		Name: "deployer does not exist",
		Key:  testKey,
	}, {
		Name: "ignore deleted deployer",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerValid).
				ObjectMeta(func(om factories.ObjectMeta) {
					om.Deleted(1)
				}).
				Get(),
		},
	}, {
		Name: "deployer get error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "Deployer"),
		},
		ShouldErr: true,
	}, {
		Name: "create resources, from application",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				ApplicationRef(testApplication.Name).
				Get(),
			testApplication,
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
			rtesting.NewTrackRequest(testApplication, deployerMinimal, scheme),
		},
		ExpectCreates: []runtime.Object{
			deploymentCreate,
			serviceCreate,
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-001", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
		},
	}, {
		Name: "create resources, from application, application not found",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				ApplicationRef(testApplication.Name).
				Get(),
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
			rtesting.NewTrackRequest(testApplication, deployerMinimal, scheme),
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionDeploymentReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionServiceReady,
						Status: corev1.ConditionUnknown,
					},
				).
				Get(),
		},
	}, {
		Name: "create resources, from application, no latest image",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				ApplicationRef(testApplication.Name).
				Get(),
			factories.Application(testApplication).
				StatusLatestImage("").
				Get(),
			testSettings,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
			rtesting.NewTrackRequest(testApplication, deployerMinimal, scheme),
		},
	}, {
		Name: "create resources, from function",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				FunctionRef(testFunction.Name).
				Get(),
			testFunction,
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
			rtesting.NewTrackRequest(testFunction, deployerMinimal, scheme),
		},
		ExpectCreates: []runtime.Object{
			deploymentCreate,
			serviceCreate,
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-001", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
		},
	}, {
		Name: "create resources, from function, function not found",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				FunctionRef(testFunction.Name).
				Get(),
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
			rtesting.NewTrackRequest(testFunction, deployerMinimal, scheme),
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionDeploymentReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionServiceReady,
						Status: corev1.ConditionUnknown,
					},
				).
				Get(),
		},
	}, {
		Name: "create resources, from function, no latest image",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				FunctionRef(testFunction.Name).
				Get(),
			factories.Function(testFunction).
				StatusLatestImage("").
				Get(),
			testSettings,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
			rtesting.NewTrackRequest(testFunction, deployerMinimal, scheme),
		},
	}, {
		Name: "create resources, from container",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				ContainerRef(testContainer.Name).
				Get(),
			testContainer,
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
			rtesting.NewTrackRequest(testContainer, deployerMinimal, scheme),
		},
		ExpectCreates: []runtime.Object{
			deploymentCreate,
			serviceCreate,
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-001", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
		},
	}, {
		Name: "create resources, from container, container not found",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				ContainerRef(testContainer.Name).
				Get(),
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
			rtesting.NewTrackRequest(testContainer, deployerMinimal, scheme),
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionDeploymentReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionServiceReady,
						Status: corev1.ConditionUnknown,
					},
				).
				Get(),
		},
	}, {
		Name: "create resources, from container, no latest image",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				ContainerRef(testContainer.Name).
				Get(),
			factories.Container(testContainer).
				StatusLatestImage("").
				Get(),
			testSettings,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
			rtesting.NewTrackRequest(testContainer, deployerMinimal, scheme),
		},
	}, {
		Name: "create resources, from image",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				Get(),
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectCreates: []runtime.Object{
			deploymentCreate,
			serviceCreate,
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-001", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
		},
	}, {
		Name: "create deployment, error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("create", "Deployment"),
		},
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				Get(),
			testSettings,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectCreates: []runtime.Object{
			deploymentCreate,
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionDeploymentReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionServiceReady,
						Status: corev1.ConditionUnknown,
					},
				).
				StatusLatestImage(testImage).
				Get(),
		},
	}, {
		Name: "create service, error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("create", "Service"),
		},
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				Get(),
			testSettings,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectCreates: []runtime.Object{
			deploymentCreate,
			serviceCreate,
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionDeploymentReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionServiceReady,
						Status: corev1.ConditionUnknown,
					},
				).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-001", deployerMinimal.Name).
				Get(),
		},
	}, {
		Name: "create service, conflicted",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("create", "Service", rtesting.InduceFailureOpts{
				Error: apierrs.NewAlreadyExists(schema.GroupResource{}, testName),
			}),
		},
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				Get(),
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectCreates: []runtime.Object{
			deploymentCreate,
			serviceCreate,
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionDeploymentReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:    corev1alpha1.DeployerConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  "NotOwned",
						Message: `There is an existing Service "test-deployer" that the Deployer does not own.`,
					},
					apis.Condition{
						Type:    corev1alpha1.DeployerConditionServiceReady,
						Status:  corev1.ConditionFalse,
						Reason:  "NotOwned",
						Message: `There is an existing Service "test-deployer" that the Deployer does not own.`,
					},
				).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-001", deployerMinimal.Name).
				Get(),
		},
	}, {
		Name: "update deployment",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
			factories.Deployment(deploymentGiven).
				HandlerContainer(func(container *corev1.Container) {
					// change to reverse
					container.Env = nil
				}).
				Get(),
			serviceGiven,
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectUpdates: []runtime.Object{
			deploymentGiven,
		},
	}, {
		Name: "update deployment, update error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("update", "Deployment"),
		},
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
			factories.Deployment(deploymentGiven).
				HandlerContainer(func(container *corev1.Container) {
					// change to reverse
					container.Env = nil
				}).
				Get(),
			serviceGiven,
			testSettings,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectUpdates: []runtime.Object{
			deploymentGiven,
		},
	}, {
		Name: "update deployment, list deployments failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("list", "DeploymentList"),
		},
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
			factories.Deployment(deploymentGiven).
				HandlerContainer(func(container *corev1.Container) {
					// change to reverse
					container.Env = nil
				}).
				Get(),
			serviceGiven,
			testSettings,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
	}, {
		Name: "update service",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
			deploymentGiven,
			factories.Service(serviceGiven).
				// change to reverse
				Ports().
				Get(),
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectUpdates: []runtime.Object{
			serviceGiven,
		},
	}, {
		Name: "update service, update error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("update", "Service"),
		},
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
			deploymentGiven,
			factories.Service(serviceGiven).
				// change to reverse
				Ports().
				Get(),
			testSettings,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectUpdates: []runtime.Object{
			serviceGiven,
		},
	}, {
		Name: "update service, list services failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("list", "ServiceList"),
		},
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
			deploymentGiven,
			factories.Service(serviceGiven).
				// change to reverse
				Ports().
				Get(),
			testSettings,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
	}, {
		Name: "cleanup extra deployments",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-001", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
			factories.Deployment(deploymentGiven).
				NamespaceName(testNamespace, "extra-deployment-1").
				Get(),
			factories.Deployment(deploymentGiven).
				NamespaceName(testNamespace, "extra-deployment-2").
				Get(),
			serviceGiven,
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectDeletes: []rtesting.DeleteRef{
			{Group: "apps", Kind: "Deployment", Namespace: testNamespace, Name: "extra-deployment-1"},
			{Group: "apps", Kind: "Deployment", Namespace: testNamespace, Name: "extra-deployment-2"},
		},
		ExpectCreates: []runtime.Object{
			deploymentCreate,
		},
	}, {
		Name: "cleanup extra deployments, delete deployment failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("delete", "Deployment"),
		},
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-001", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
			factories.Deployment(deploymentGiven).
				NamespaceName(testNamespace, "extra-deployment-1").
				Get(),
			factories.Deployment(deploymentGiven).
				NamespaceName(testNamespace, "extra-deployment-2").
				Get(),
			serviceGiven,
			testSettings,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectDeletes: []rtesting.DeleteRef{
			{Group: "apps", Kind: "Deployment", Namespace: testNamespace, Name: "extra-deployment-1"},
		},
	}, {
		Name: "cleanup extra services",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
			deploymentGiven,
			factories.Service(serviceGiven).
				NamespaceName(testNamespace, "extra-service-1").
				Get(),
			factories.Service(serviceGiven).
				NamespaceName(testNamespace, "extra-service-2").
				Get(),
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectDeletes: []rtesting.DeleteRef{
			{Kind: "Service", Namespace: testNamespace, Name: "extra-service-1"},
			{Kind: "Service", Namespace: testNamespace, Name: "extra-service-2"},
		},
		ExpectCreates: []runtime.Object{
			serviceCreate,
		},
	}, {
		Name: "cleanup extra services, delete service failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("delete", "Service"),
		},
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
			deploymentGiven,
			factories.Service(serviceGiven).
				NamespaceName(testNamespace, "extra-service-1").
				Get(),
			factories.Service(serviceGiven).
				NamespaceName(testNamespace, "extra-service-2").
				Get(),
			testSettings,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectDeletes: []rtesting.DeleteRef{
			{Kind: "Service", Namespace: testNamespace, Name: "extra-service-1"},
		},
	}, {
		Name: "create ingress",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				IngressPolicy(corev1alpha1.IngressPolicyExternal).
				Get(),
			deploymentGiven,
			serviceGiven,
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectCreates: []runtime.Object{
			ingressCreate,
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(deployerDefaultConditions(
					apis.Condition{
						Type:     corev1alpha1.DeployerConditionIngressReady,
						Status:   corev1.ConditionUnknown,
						Severity: apis.ConditionSeverityInfo,
						Reason:   "IngressNotConfigured",
						Message:  "Ingress has not yet been reconciled.",
					},
				)...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusIngressRef("%s-deployer-001", testName).
				StatusAddressURL(testAddressURL).
				StatusURL(testURL).
				Get(),
		},
	}, {
		Name: "create ingress, create failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("create", "Ingress"),
		},
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				IngressPolicy(corev1alpha1.IngressPolicyExternal).
				Get(),
			deploymentGiven,
			serviceGiven,
			testSettings,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectCreates: []runtime.Object{
			ingressCreate,
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionDeploymentReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionServiceReady,
						Status: corev1.ConditionTrue,
					},
				).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL(testAddressURL).
				Get(),
		},
	}, {
		Name: "delete ingress",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				IngressPolicy(corev1alpha1.IngressPolicyClusterLocal).
				Get(),
			deploymentGiven,
			serviceGiven,
			ingressGiven,
			factories.ConfigMap(testSettings).
				AddData("defaultDomain", "not.example.com").
				Get(),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectDeletes: []rtesting.DeleteRef{
			{Group: "networking.k8s.io", Kind: "Ingress", Namespace: testNamespace, Name: ingressGiven.Name},
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL(testAddressURL).
				Get(),
		},
	}, {
		Name: "delete ingress, delete failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("delete", "Ingress"),
		},
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				IngressPolicy(corev1alpha1.IngressPolicyClusterLocal).
				Get(),
			deploymentGiven,
			serviceGiven,
			ingressGiven,
			factories.ConfigMap(testSettings).
				AddData("defaultDomain", "not.example.com").
				Get(),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectDeletes: []rtesting.DeleteRef{
			{Group: "networking.k8s.io", Kind: "Ingress", Namespace: testNamespace, Name: ingressGiven.Name},
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionDeploymentReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionServiceReady,
						Status: corev1.ConditionTrue,
					},
				).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL(testAddressURL).
				Get(),
		},
	}, {
		Name: "update ingress",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				IngressPolicy(corev1alpha1.IngressPolicyExternal).
				Get(),
			deploymentGiven,
			serviceGiven,
			ingressGiven,
			factories.ConfigMap(testSettings).
				AddData("defaultDomain", "not.example.com").
				Get(),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectUpdates: []runtime.Object{
			factories.Ingress(ingressGiven).
				HostToService(fmt.Sprintf("%s.%s.%s", testName, testNamespace, "not.example.com"), serviceGiven.Name).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(deployerDefaultConditions(
					apis.Condition{
						Type:     corev1alpha1.DeployerConditionIngressReady,
						Status:   corev1.ConditionUnknown,
						Severity: apis.ConditionSeverityInfo,
						Reason:   "IngressNotConfigured",
						Message:  "Ingress has not yet been reconciled.",
					},
				)...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusIngressRef("%s-deployer-000", testName).
				StatusAddressURL(testAddressURL).
				StatusURL("http://%s.%s.%s", testName, testNamespace, "not.example.com").
				Get(),
		},
	}, {
		Name: "update ingress, update failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("update", "Ingress"),
		},
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				IngressPolicy(corev1alpha1.IngressPolicyExternal).
				Get(),
			deploymentGiven,
			serviceGiven,
			ingressGiven,
			factories.ConfigMap(testSettings).
				AddData("defaultDomain", "not.example.com").
				Get(),
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectUpdates: []runtime.Object{
			factories.Ingress(ingressGiven).
				HostToService(fmt.Sprintf("%s.%s.%s", testName, testNamespace, "not.example.com"), serviceGiven.Name).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionDeploymentReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionServiceReady,
						Status: corev1.ConditionTrue,
					},
				).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL(testAddressURL).
				Get(),
		},
	}, {
		Name: "remove extra ingress",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				IngressPolicy(corev1alpha1.IngressPolicyExternal).
				Get(),
			deploymentGiven,
			serviceGiven,
			factories.Ingress(ingressGiven).
				NamespaceName(testNamespace, "extra-ingress-1").
				Get(),
			factories.Ingress(ingressGiven).
				NamespaceName(testNamespace, "extra-ingress-2").
				Get(),
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectDeletes: []rtesting.DeleteRef{
			{Group: "networking.k8s.io", Kind: "Ingress", Namespace: testNamespace, Name: "extra-ingress-1"},
			{Group: "networking.k8s.io", Kind: "Ingress", Namespace: testNamespace, Name: "extra-ingress-2"},
		},
		ExpectCreates: []runtime.Object{
			ingressCreate,
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(deployerDefaultConditions(
					apis.Condition{
						Type:     corev1alpha1.DeployerConditionIngressReady,
						Status:   corev1.ConditionUnknown,
						Severity: apis.ConditionSeverityInfo,
						Reason:   "IngressNotConfigured",
						Message:  "Ingress has not yet been reconciled.",
					},
				)...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusIngressRef("%s-deployer-001", testName).
				StatusAddressURL(testAddressURL).
				StatusURL(testURL).
				Get(),
		},
	}, {
		Name: "remove extra ingress, listing failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("list", "IngressList"),
		},
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				IngressPolicy(corev1alpha1.IngressPolicyExternal).
				Get(),
			deploymentGiven,
			serviceGiven,
			factories.Ingress(ingressGiven).
				NamespaceName(testNamespace, "extra-ingress-1").
				Get(),
			factories.Ingress(ingressGiven).
				NamespaceName(testNamespace, "extra-ingress-2").
				Get(),
			testSettings,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionDeploymentReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionServiceReady,
						Status: corev1.ConditionTrue,
					},
				).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
		},
	}, {
		Name: "remove extra ingress, delete failed",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("delete", "Ingress"),
		},
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				Image(testImage).
				IngressPolicy(corev1alpha1.IngressPolicyExternal).
				Get(),
			deploymentGiven,
			serviceGiven,
			factories.Ingress(ingressGiven).
				NamespaceName(testNamespace, "extra-ingress-1").
				Get(),
			factories.Ingress(ingressGiven).
				NamespaceName(testNamespace, "extra-ingress-2").
				Get(),
			testSettings,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectDeletes: []rtesting.DeleteRef{
			{Group: "networking.k8s.io", Kind: "Ingress", Namespace: testNamespace, Name: "extra-ingress-1"},
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionDeploymentReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionServiceReady,
						Status: corev1.ConditionTrue,
					},
				).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
		},
	}, {
		Name: "propagate labels",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddLabel(testLabelKey, testLabelValue)
				}).
				IngressPolicy(corev1alpha1.IngressPolicyExternal).
				Image(testImage).
				StatusConditions(deployerDefaultConditions(
					apis.Condition{
						Type:     corev1alpha1.DeployerConditionIngressReady,
						Status:   corev1.ConditionUnknown,
						Severity: apis.ConditionSeverityInfo,
						Reason:   "IngressNotConfigured",
						Message:  "Ingress has not yet been reconciled.",
					},
				)...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusIngressRef(ingressGiven.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				StatusURL(testURL).
				Get(),
			deploymentGiven,
			serviceGiven,
			ingressGiven,
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectUpdates: []runtime.Object{
			factories.Deployment(deploymentGiven).
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddLabel(testLabelKey, testLabelValue)
				}).
				PodTemplateSpec(func(pts factories.PodTemplateSpec) {
					pts.AddLabel(testLabelKey, testLabelValue)
				}).
				Get(),
			factories.Service(serviceGiven).
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddLabel(testLabelKey, testLabelValue)
				}).
				Get(),
			factories.Ingress(ingressGiven).
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddLabel(testLabelKey, testLabelValue)
				}).
				Get(),
		},
	}, {
		Name: "ready",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			deployerValid,
			factories.Deployment(deploymentGiven).
				StatusConditions(
					apis.Condition{
						Type:   "Available",
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   "Progressing",
						Status: corev1.ConditionUnknown,
					},
				).
				Get(),
			serviceGiven,
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(deployerDefaultConditions(
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionDeploymentReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionReady,
						Status: corev1.ConditionTrue,
					},
				)...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
		},
	}, {
		Name: "ready, with ingress",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			factories.DeployerCore(deployerValid).
				IngressPolicy(corev1alpha1.IngressPolicyExternal).
				Get(),
			factories.Deployment(deploymentGiven).
				StatusConditions(
					apis.Condition{
						Type:   "Available",
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   "Progressing",
						Status: corev1.ConditionUnknown,
					},
				).
				Get(),
			serviceGiven,
			factories.Ingress(ingressGiven).
				StatusLoadBalancer(
					corev1.LoadBalancerIngress{
						Hostname: testHost,
					},
				).
				Get(),
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(deployerDefaultConditions(
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionDeploymentReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:     corev1alpha1.DeployerConditionIngressReady,
						Status:   corev1.ConditionTrue,
						Severity: apis.ConditionSeverityInfo,
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionReady,
						Status: corev1.ConditionTrue,
					},
				)...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusIngressRef("%s-deployer-000", deployerMinimal.Name).
				StatusAddressURL(testAddressURL).
				StatusURL(testURL).
				Get(),
		},
	}, {
		Name: "not ready",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			deployerValid,
			factories.Deployment(deploymentGiven).
				StatusConditions(
					apis.Condition{
						Type:    "Available",
						Status:  corev1.ConditionFalse,
						Reason:  testConditionReason,
						Message: testConditionMessage,
					},
					apis.Condition{
						Type:   "Progressing",
						Status: corev1.ConditionUnknown,
					},
				).
				Get(),
			serviceGiven,
			testSettings,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(deployerDefaultConditions(
					apis.Condition{
						Type:    corev1alpha1.DeployerConditionDeploymentReady,
						Status:  corev1.ConditionFalse,
						Reason:  testConditionReason,
						Message: testConditionMessage,
					},
					apis.Condition{
						Type:    corev1alpha1.DeployerConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  testConditionReason,
						Message: testConditionMessage,
					},
				)...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
		},
	}, {
		Name: "update status error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("update", "Deployer"),
		},
		GivenObjects: []runtime.Object{
			deployerValid,
			deploymentGiven,
			serviceGiven,
			testSettings,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(deployerDefaultConditions()...).
				StatusLatestImage(testImage).
				StatusDeploymentRef("%s-deployer-000", deployerMinimal.Name).
				StatusServiceRef(deployerMinimal.Name).
				StatusAddressURL("http://%s.%s.svc.cluster.local", serviceCreate.Name, serviceCreate.Namespace).
				Get(),
		},
	}, {
		Name: "settings not found",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			deployerMinimal,
		},
		ShouldErr: true,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(testSettings, deployerMinimal, scheme),
		},
		ExpectStatusUpdates: []runtime.Object{
			factories.DeployerCore(deployerMinimal).
				StatusConditions(
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionDeploymentReady,
						Status: "Unknown",
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionReady,
						Status: "Unknown",
					},
					apis.Condition{
						Type:   corev1alpha1.DeployerConditionServiceReady,
						Status: "Unknown",
					},
				).
				Get(),
		},
	}}

	table.Test(t, scheme, func(t *testing.T, row *rtesting.Testcase, client client.Client, tracker tracker.Tracker, log logr.Logger) reconcile.Reconciler {
		return &core.DeployerReconciler{
			Client:  client,
			Scheme:  scheme,
			Log:     log,
			Tracker: tracker,
		}
	})
}

func deployerDefaultConditions(conditions ...apis.Condition) []apis.Condition {
	defaults := map[apis.ConditionType]apis.Condition{
		corev1alpha1.DeployerConditionDeploymentReady: {
			Type:   corev1alpha1.DeployerConditionDeploymentReady,
			Status: corev1.ConditionUnknown,
		},
		corev1alpha1.DeployerConditionIngressReady: {
			Type:     corev1alpha1.DeployerConditionIngressReady,
			Status:   corev1.ConditionFalse,
			Severity: apis.ConditionSeverityInfo,
			Reason:   "IngressNotRequired",
			Message:  "Ingress resource is not required.",
		},
		corev1alpha1.DeployerConditionReady: {
			Type:   corev1alpha1.DeployerConditionReady,
			Status: corev1.ConditionUnknown,
		},
		corev1alpha1.DeployerConditionServiceReady: {
			Type:   corev1alpha1.DeployerConditionServiceReady,
			Status: corev1.ConditionTrue,
		},
	}

	for _, condition := range conditions {
		defaults[condition.Type] = condition
	}

	return []apis.Condition{
		defaults[corev1alpha1.DeployerConditionDeploymentReady],
		defaults[corev1alpha1.DeployerConditionIngressReady],
		defaults[corev1alpha1.DeployerConditionReady],
		defaults[corev1alpha1.DeployerConditionServiceReady],
	}
}
