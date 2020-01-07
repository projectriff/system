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
	"errors"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/projectriff/system/pkg/apis"
	"github.com/projectriff/system/pkg/apis/build/v1alpha1"
	corev1alpha1 "github.com/projectriff/system/pkg/apis/core/v1alpha1"
	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
	kedav1alpha1 "github.com/projectriff/system/pkg/apis/thirdparty/keda/v1alpha1"
	rtesting "github.com/projectriff/system/pkg/controllers/testing"
	"github.com/projectriff/system/pkg/refs"
	"github.com/projectriff/system/pkg/tracker"
)

const (
	testNamespace      = "test-namespace"
	testName           = "test-processor"
	testProcessorImage = "test-processor-image"
	testDefaultImage   = "test-image-from-template"
	testFunction       = "test-function"
	testFunctionImage  = "test-function-image"
	testContainer      = "test-container"
	testContainerImage = "test-container-image"
	testErrorMessage   = "test error"
)

var testError = errors.New(testErrorMessage)

func TestReconcile(t *testing.T) {
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
		GivenObjects: []runtime.Object{
			processor(testNamespace, testName),
		},
		ShouldErr: true,
		Verify:    rtesting.AssertErrorMessagef("configmaps %q not found", processorImages),
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.CreateTrackRequest("", "ConfigMap", "", processorImages).By(testNamespace, testName),
		},
		ExpectStatusUpdates: []runtime.Object{
			processorStatusConditionsUnknown(testNamespace, testName),
		},
	}, {
		Name: "processor is marked for deletion",
		GivenObjects: []runtime.Object{
			processor(testNamespace, testName, func(proc *streamingv1alpha1.Processor) {
				proc.ObjectMeta.DeletionTimestamp = now()
			}),
		},
		Key: types.NamespacedName{Namespace: testNamespace, Name: testName},
	}, {
		Name: "getting configMap fails",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []runtime.Object{
			processor(testNamespace, testName),
		},
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "ConfigMap"),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.CreateTrackRequest("", "ConfigMap", "", processorImages).By(testNamespace, testName),
		},
		ShouldErr: true,
		ExpectStatusUpdates: []runtime.Object{
			processorStatusConditionsUnknown(testNamespace, testName),
		},
	}, {
		Name: "processor sidecar image not present in configMap",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []runtime.Object{
			processor(testNamespace, testName),
			configMap(testNamespace, processorImages),
		},
		ShouldErr: true,
		Verify:    rtesting.AssertErrorMessagef("missing processor image configuration"),
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.CreateTrackRequest("", "ConfigMap", "", processorImages).By(testNamespace, testName),
		},
		ExpectStatusUpdates: []runtime.Object{
			processorStatusConditionsUnknown(testNamespace, testName,
				setProcessorStatusLatestImage(testDefaultImage),
			),
		},
	}, {
		Name: "default application image not set",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []runtime.Object{
			processor(testNamespace, testName, func(proc *streamingv1alpha1.Processor) {
				proc.Spec.Template = nil
			}),
			configMap(testNamespace, processorImages, map[string]string{processorImageKey: testProcessorImage}),
		},
		ShouldErr: true,
		Verify:    rtesting.AssertErrorMessagef("could not resolve an image"),
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.CreateTrackRequest("", "ConfigMap", "", processorImages).By(testNamespace, testName),
		},
		ExpectStatusUpdates: []runtime.Object{
			processorStatusConditionsUnknown(testNamespace, testName),
		},
	}, {
		Name: "successful reconciliation",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []runtime.Object{
			processor(testNamespace, testName),
			configMap(testNamespace, processorImages, map[string]string{processorImageKey: testProcessorImage}),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.CreateTrackRequest("", "ConfigMap", "", processorImages).By(testNamespace, testName),
		},
		ExpectCreates: []runtime.Object{
			processorDeployment(testNamespace, testName, testDefaultImage),
			processorScaledObject(testNamespace, testName),
		},
		ExpectStatusUpdates: []runtime.Object{
			processorStatusConditionsUnknown(testNamespace, testName,
				setProcessorConditionsTrue("StreamsReady", "ScaledObjectReady"),
				setProcessorStatusLatestImage(testDefaultImage),
				setProcessorStatusDeploymentRef(testName+"-processor-001"),
				setProcessorStatusScaledObjectRef(testName+"-processor-002"),
			),
		},
	}, {
		Name: "deployment creation fails",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []runtime.Object{
			processor(testNamespace, testName),
			configMap(testNamespace, processorImages, map[string]string{processorImageKey: testProcessorImage}),
		},
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("create", "Deployment"),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.CreateTrackRequest("", "ConfigMap", "", processorImages).By(testNamespace, testName),
		},
		ExpectCreates: []runtime.Object{
			processorDeployment(testNamespace, testName, testDefaultImage),
		},
		ShouldErr: true,
		ExpectStatusUpdates: []runtime.Object{
			processorStatusConditionsUnknown(testNamespace, testName,
				setProcessorStatusLatestImage(testDefaultImage),
			),
		},
	}, {
		Name: "scaled object creation fails",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []runtime.Object{
			processor(testNamespace, testName),
			configMap(testNamespace, processorImages, map[string]string{processorImageKey: testProcessorImage}),
		},
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("create", "ScaledObject"),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.CreateTrackRequest("", "ConfigMap", "", processorImages).By(testNamespace, testName),
		},
		ExpectCreates: []runtime.Object{
			processorDeployment(testNamespace, testName, testDefaultImage),
			processorScaledObject(testNamespace, testName),
		},
		ShouldErr: true,
		ExpectStatusUpdates: []runtime.Object{
			processorStatusConditionsUnknown(testNamespace, testName,
				setProcessorConditionsTrue("StreamsReady"),
				setProcessorStatusLatestImage(testDefaultImage),
				setProcessorStatusDeploymentRef(testName+"-processor-001"),
			),
		},
	}, {
		Name: "successful reconciliation with unsatisfied function reference",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []runtime.Object{
			processor(testNamespace, testName, functionDelta),
			configMap(testNamespace, processorImages, map[string]string{processorImageKey: testProcessorImage}),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.CreateTrackRequest("", "ConfigMap", "", processorImages).By(testNamespace, testName),
			rtesting.CreateTrackRequest("build.projectriff.io", "Function", testNamespace, testFunction).By(testNamespace, testName),
		},
		ExpectStatusUpdates: []runtime.Object{
			processorStatusConditionsUnknown(testNamespace, testName),
		},
	}, {
		Name: "get function fails",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []runtime.Object{
			processor(testNamespace, testName, functionDelta),
			configMap(testNamespace, processorImages, map[string]string{processorImageKey: testProcessorImage}),
		},
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "Function"),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.CreateTrackRequest("", "ConfigMap", "", processorImages).By(testNamespace, testName),
			rtesting.CreateTrackRequest("build.projectriff.io", "Function", testNamespace, testFunction).By(testNamespace, testName),
		},
		ShouldErr: true,
	}, {
		Name: "successful reconciliation with satisfied function reference",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []runtime.Object{
			processor(testNamespace, testName, functionDelta),
			configMap(testNamespace, processorImages, map[string]string{processorImageKey: testProcessorImage}),
			function(testNamespace),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.CreateTrackRequest("", "ConfigMap", "", processorImages).By(testNamespace, testName),
			rtesting.CreateTrackRequest("build.projectriff.io", "Function", testNamespace, testFunction).By(testNamespace, testName),
		},
		ExpectCreates: []runtime.Object{
			processorDeployment(testNamespace, testName, testFunctionImage),
			processorScaledObject(testNamespace, testName),
		},
		ExpectStatusUpdates: []runtime.Object{
			processorStatusConditionsUnknown(testNamespace, testName,
				setProcessorConditionsTrue("StreamsReady", "ScaledObjectReady"),
				setProcessorStatusLatestImage(testFunctionImage),
				setProcessorStatusDeploymentRef(testName+"-processor-001"),
				setProcessorStatusScaledObjectRef(testName+"-processor-002"),
			),
		},
	}, {
		Name: "successful reconciliation with unsatisfied container reference",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []runtime.Object{
			processor(testNamespace, testName, containerDelta),
			configMap(testNamespace, processorImages, map[string]string{processorImageKey: testProcessorImage}),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.CreateTrackRequest("", "ConfigMap", "", processorImages).By(testNamespace, testName),
			rtesting.CreateTrackRequest("build.projectriff.io", "Container", testNamespace, testContainer).By(testNamespace, testName),
		},
		ExpectStatusUpdates: []runtime.Object{
			processorStatusConditionsUnknown(testNamespace, testName),
		},
	}, {
		Name: "get container fails",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []runtime.Object{
			processor(testNamespace, testName, containerDelta),
			configMap(testNamespace, processorImages, map[string]string{processorImageKey: testProcessorImage}),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.CreateTrackRequest("", "ConfigMap", "", processorImages).By(testNamespace, testName),
			rtesting.CreateTrackRequest("build.projectriff.io", "Container", testNamespace, testContainer).By(testNamespace, testName),
		},
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "Container"),
		},
		ShouldErr: true,
	}, {
		Name: "successful reconciliation with satisfied container reference",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: testName},
		GivenObjects: []runtime.Object{
			processor(testNamespace, testName, containerDelta),
			configMap(testNamespace, processorImages, map[string]string{processorImageKey: testProcessorImage}),
			container(testNamespace),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.CreateTrackRequest("", "ConfigMap", "", processorImages).By(testNamespace, testName),
			rtesting.CreateTrackRequest("build.projectriff.io", "Container", testNamespace, testContainer).By(testNamespace, testName),
		},
		ExpectCreates: []runtime.Object{
			processorDeployment(testNamespace, testName, testContainerImage),
			processorScaledObject(testNamespace, testName),
		},
		ExpectStatusUpdates: []runtime.Object{
			processorStatusConditionsUnknown(testNamespace, testName,
				setProcessorConditionsTrue("StreamsReady", "ScaledObjectReady"),
				setProcessorStatusLatestImage(testContainerImage),
				setProcessorStatusDeploymentRef(testName+"-processor-001"),
				setProcessorStatusScaledObjectRef(testName+"-processor-002"),
			),
		},
	}}

	table.Test(t, scheme, func(t *testing.T, row *rtesting.Testcase, client client.Client, tracker tracker.Tracker, log logr.Logger) reconcile.Reconciler {
		return &ProcessorReconciler{
			Client:  client,
			Tracker: tracker,
			Scheme:  scheme,
			Log:     log,
		}
	})
}

var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = corev1alpha1.AddToScheme(scheme)
	_ = streamingv1alpha1.AddToScheme(scheme)
	_ = kedav1alpha1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
}

func processor(namespace, name string, delta ...func(*streamingv1alpha1.Processor)) *streamingv1alpha1.Processor {
	proc := &streamingv1alpha1.Processor{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: streamingv1alpha1.ProcessorSpec{
			Template: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Image: testDefaultImage}},
				},
			},
		},
	}
	for _, d := range delta {
		d(proc)
	}
	return proc
}

func functionDelta(proc *streamingv1alpha1.Processor) {
	proc.Spec.Build = &streamingv1alpha1.Build{
		FunctionRef: testFunction,
	}
}

func containerDelta(proc *streamingv1alpha1.Processor) {
	proc.Spec.Build = &streamingv1alpha1.Build{
		ContainerRef: testContainer,
	}
}

func setProcessorStatusLatestImage(image string) func(proc *streamingv1alpha1.Processor) {
	return func(proc *streamingv1alpha1.Processor) {
		proc.Status.LatestImage = image
	}
}

func setProcessorStatusDeploymentRef(deploymentName string) func(proc *streamingv1alpha1.Processor) {
	return func(proc *streamingv1alpha1.Processor) {
		proc.Status.DeploymentRef = &refs.TypedLocalObjectReference{
			APIGroup: rtesting.StringPtr("apps"),
			Kind:     "Deployment",
			Name:     deploymentName,
		}
	}
}

func setProcessorStatusScaledObjectRef(deploymentName string) func(proc *streamingv1alpha1.Processor) {
	return func(proc *streamingv1alpha1.Processor) {
		proc.Status.ScaledObjectRef = &refs.TypedLocalObjectReference{
			APIGroup: rtesting.StringPtr("keda.k8s.io"),
			Kind:     "ScaledObject",
			Name:     deploymentName,
		}
	}
}

func setProcessorConditionsTrue(conditionTypes ...apis.ConditionType) func(proc *streamingv1alpha1.Processor) {
	return func(proc *streamingv1alpha1.Processor) {
		for _, conditionType := range conditionTypes {
			proc.Status.Conditions = setConditionTrue(proc.Status.Conditions, conditionType)
		}
	}
}

func setConditionTrue(input []apis.Condition, conditionType apis.ConditionType) []apis.Condition {
	output := []apis.Condition{}
	for _, c := range input {
		d := c
		if d.Type == conditionType {
			d.Status = corev1.ConditionTrue
		}
		output = append(output, d)
	}
	return output
}

func processorStatusConditionsUnknown(namespace string, name string, delta ...func(*streamingv1alpha1.Processor)) *streamingv1alpha1.Processor {
	deltas := []func(proc *streamingv1alpha1.Processor){
		func(proc *streamingv1alpha1.Processor) {
			proc.Status.Conditions = []apis.Condition{{
				Type:   "DeploymentReady",
				Status: "Unknown",
			}, {
				Type:   "Ready",
				Status: "Unknown",
			}, {
				Type:   "ScaledObjectReady",
				Status: "Unknown",
			}, {
				Type:   "StreamsReady",
				Status: "Unknown",
			}}
		}}
	deltas = append(deltas, delta...)
	return processor(namespace, name, deltas...)
}

func configMap(namespace, name string, data ...map[string]string) *corev1.ConfigMap {
	if len(data) > 1 {
		panic("configMap takes at most one data map")
	}
	if len(data) == 0 {
		data = []map[string]string{{}}
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Data: data[0],
	}
}

func function(namespace string) *v1alpha1.Function {
	return &v1alpha1.Function{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      testFunction,
		},
		Status: v1alpha1.FunctionStatus{
			BuildStatus: v1alpha1.BuildStatus{
				LatestImage: testFunctionImage,
			},
		},
	}
}

func container(namespace string) *v1alpha1.Container {
	return &v1alpha1.Container{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      testContainer,
		},
		Status: v1alpha1.ContainerStatus{
			BuildStatus: v1alpha1.BuildStatus{
				LatestImage: testContainerImage,
			},
		},
	}
}

func processorDeployment(namespace, processorName, imageRef string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: processorName + "-processor-",
			Namespace:    namespace,
			Labels:       map[string]string{"streaming.projectriff.io/processor": processorName},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         "streaming.projectriff.io/v1alpha1",
				Kind:               "Processor",
				Name:               processorName,
				Controller:         truePtr(),
				BlockOwnerDeletion: truePtr(),
			}},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"streaming.projectriff.io/processor": processorName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"streaming.projectriff.io/processor": processorName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "function",
						Image: imageRef,
						Ports: []corev1.ContainerPort{{ContainerPort: 8081}},
					}, {
						Name:  "processor",
						Image: testProcessorImage,
						Env: []corev1.EnvVar{
							{Name: "CNB_BINDINGS", Value: "/var/riff/bindings"},
							{Name: "INPUT_START_OFFSETS"},
							{Name: "INPUT_NAMES"},
							{Name: "OUTPUT_NAMES"},
							{Name: "GROUP", Value: processorName},
							{Name: "FUNCTION", Value: "localhost:8081"},
						},
						ImagePullPolicy: "IfNotPresent",
					}},
				},
			},
		},
	}
}

func processorScaledObject(namespace, processorName string) runtime.Object {
	return &kedav1alpha1.ScaledObject{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: processorName + "-processor-",
			Namespace:    namespace,
			Labels:       map[string]string{"streaming.projectriff.io/processor": processorName, "deploymentName": processorName + "-processor-001"},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         "streaming.projectriff.io/v1alpha1",
				Kind:               "Processor",
				Name:               processorName,
				Controller:         truePtr(),
				BlockOwnerDeletion: truePtr(),
			}},
		},
		Spec: kedav1alpha1.ScaledObjectSpec{
			ScaleTargetRef:  &kedav1alpha1.ObjectReference{DeploymentName: processorName + "-processor-001"},
			PollingInterval: int32Ptr(1),
			CooldownPeriod:  int32Ptr(30),
			MinReplicaCount: int32Ptr(1),
			MaxReplicaCount: int32Ptr(30),
		},
	}
}

func int32Ptr(v int32) *int32 {
	return &v
}

func truePtr() *bool {
	v := true
	return &v
}

func now() *metav1.Time {
	t := metav1.Now()
	return &t
}
