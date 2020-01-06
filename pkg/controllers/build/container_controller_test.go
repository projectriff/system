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

package build_test

import (
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	kpackbuildv1alpha1 "github.com/projectriff/system/pkg/apis/thirdparty/kpack/build/v1alpha1"
	"github.com/projectriff/system/pkg/controllers/build"
	rtesting "github.com/projectriff/system/pkg/controllers/testing"
	"github.com/projectriff/system/pkg/controllers/testing/factories"
	"github.com/projectriff/system/pkg/tracker"
)

func TestContainerReconciler(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-container"
	testKey := types.NamespacedName{Namespace: testNamespace, Name: testName}
	testImagePrefix := "example.com/repo"
	testSha256 := "cf8b4c69d5460f88530e1c80b8856a70801f31c50b191c8413043ba9b160a43e"

	containerConditionImageResolved := factories.Condition().Type(buildv1alpha1.ContainerConditionImageResolved)
	containerConditionReady := factories.Condition().Type(buildv1alpha1.ContainerConditionReady)

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = kpackbuildv1alpha1.AddToScheme(scheme)
	_ = buildv1alpha1.AddToScheme(scheme)

	containerMinimal := factories.Container().
		NamespaceName(testNamespace, testName)
	containerValid := containerMinimal.
		Image("%s/%s", testImagePrefix, testName)

	cmImagePrefix := factories.ConfigMap().
		NamespaceName(testNamespace, "riff-build").
		AddData("default-image-prefix", "")

	serviceAccount := factories.ServiceAccount().
		NamespaceName(testNamespace, "riff-build")

	table := rtesting.Table{{
		Name: "container does not exist",
		Key:  testKey,
	}, {
		Name: "ignore deleted container",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			containerValid.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.Deleted(1)
				}).
				Get(),
		},
	}, {
		// TODO mock image digest resolution
		Skip: true,
		Name: "resolve images digest",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			containerValid.Get(),
			serviceAccount.Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			containerMinimal.
				StatusConditions(
					containerConditionImageResolved.True(),
					containerConditionReady.Unknown(),
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				StatusLatestImage("%s/%s@sha256:%s", testImagePrefix, testName, testSha256).
				Get(),
		},
	}, {
		Name: "container get error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "Container"),
		},
		ShouldErr: true,
	}, {
		// TODO mock image digest resolution
		Skip: true,
		Name: "default image",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			cmImagePrefix.
				AddData("default-image-prefix", testImagePrefix).
				Get(),
			containerMinimal.Get(),
			serviceAccount.Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			containerMinimal.
				StatusConditions(
					containerConditionImageResolved.True(),
					containerConditionReady.Unknown(),
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
	}, {
		Name: "default image, missing",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			containerMinimal.Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			containerMinimal.
				StatusConditions(
					containerConditionImageResolved.False().Reason("DefaultImagePrefixMissing", "missing default image prefix"),
					containerConditionReady.False().Reason("DefaultImagePrefixMissing", "missing default image prefix"),
				).
				Get(),
		},
		ShouldErr: true,
	}, {
		Name: "default image, undefined",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			cmImagePrefix.Get(),
			containerMinimal.Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			containerMinimal.
				StatusConditions(
					containerConditionImageResolved.False().Reason("DefaultImagePrefixMissing", "missing default image prefix"),
					containerConditionReady.False().Reason("DefaultImagePrefixMissing", "missing default image prefix"),
				).
				Get(),
		},
		ShouldErr: true,
	}, {
		Name: "default image, error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "ConfigMap"),
		},
		GivenObjects: []runtime.Object{
			cmImagePrefix.Get(),
			containerMinimal.Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			containerMinimal.
				StatusConditions(
					containerConditionImageResolved.False().Reason("ImageInvalid", "inducing failure for get ConfigMap"),
					containerConditionReady.False().Reason("ImageInvalid", "inducing failure for get ConfigMap"),
				).
				Get(),
		},
		ShouldErr: true,
	}, {
		// TODO mock image digest resolution
		Skip: true,
		Name: "container status update error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("update", "Container"),
		},
		GivenObjects: []runtime.Object{
			containerValid.Get(),
			serviceAccount.Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			containerMinimal.
				StatusConditions(
					containerConditionImageResolved.True(),
					containerConditionReady.Unknown(),
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
		ShouldErr: true,
	}}

	table.Test(t, scheme, func(t *testing.T, row *rtesting.Testcase, client client.Client, tracker tracker.Tracker, log logr.Logger) reconcile.Reconciler {
		return &build.ContainerReconciler{
			Client: client,
			Scheme: scheme,
			Log:    log,
		}
	})
}