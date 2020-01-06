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

	kpackbuildv1alpha1 "github.com/projectriff/system/pkg/apis/thirdparty/kpack/build/v1alpha1"
	"github.com/projectriff/system/pkg/controllers/build"
	rtesting "github.com/projectriff/system/pkg/controllers/testing"
	"github.com/projectriff/system/pkg/controllers/testing/factories"
	"github.com/projectriff/system/pkg/tracker"
)

func TestClusterBuildersReconcile(t *testing.T) {
	testNamespace := "riff-system"
	testName := "builders"
	testKey := types.NamespacedName{Namespace: testNamespace, Name: testName}
	testApplicationImage := "projectriff/builder:application"
	testFunctionImage := "projectriff/builder:function"

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = kpackbuildv1alpha1.AddToScheme(scheme)

	testApplicationBuilder := factories.KpackClusterBuilder().
		NamespaceName("", "riff-application").
		Image(testApplicationImage)
	testApplicationBuilderReady := testApplicationBuilder.
		StatusReady().
		StatusLatestImage(testApplicationImage)
	testFunctionBuilder := factories.KpackClusterBuilder().
		NamespaceName("", "riff-function").
		Image(testFunctionImage)
	testFunctionBuilderReady := testFunctionBuilder.
		StatusReady().
		StatusLatestImage(testFunctionImage)

	testBuilders := factories.ConfigMap().
		NamespaceName(testNamespace, testName)

	table := rtesting.Table{{
		Name: "builders configmap does not exist",
		Key:  testKey,
		ExpectCreates: []runtime.Object{
			testBuilders.Get(),
		},
	}, {
		Name: "builders configmap unchanged",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testBuilders.Get(),
		},
	}, {
		Name: "ignore deleted builders configmap",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testBuilders.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.Deleted(1)
				}).
				Get(),
			testApplicationBuilder.Get(),
			testFunctionBuilder.Get(),
		},
	}, {
		Name: "ignore other configmaps in the correct namespace",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: "not-builders"},
		GivenObjects: []runtime.Object{
			testBuilders.
				NamespaceName(testNamespace, "not-builders").
				Get(),
			testApplicationBuilder.Get(),
			testFunctionBuilder.Get(),
		},
	}, {
		Name: "ignore other configmaps in the wrong namespace",
		Key:  types.NamespacedName{Namespace: "not-riff-system", Name: testName},
		GivenObjects: []runtime.Object{
			testBuilders.
				NamespaceName("not-riff-system", testName).
				Get(),
			testApplicationBuilder.Get(),
			testFunctionBuilder.Get(),
		},
	}, {
		Name: "create builders configmap, not ready",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testApplicationBuilder.Get(),
			testFunctionBuilder.Get(),
		},
		ExpectCreates: []runtime.Object{
			testBuilders.
				AddData("riff-application", "").
				AddData("riff-function", "").
				Get(),
		},
	}, {
		Name: "create builders configmap, ready",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testApplicationBuilderReady.Get(),
			testFunctionBuilderReady.Get(),
		},
		ExpectCreates: []runtime.Object{
			testBuilders.
				AddData("riff-application", testApplicationImage).
				AddData("riff-function", testFunctionImage).
				Get(),
		},
	}, {
		Name: "create builders configmap, error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("create", "ConfigMap"),
		},
		GivenObjects: []runtime.Object{
			testApplicationBuilder.Get(),
			testFunctionBuilder.Get(),
		},
		ShouldErr: true,
		ExpectCreates: []runtime.Object{
			testBuilders.
				AddData("riff-application", "").
				AddData("riff-function", "").
				Get(),
		},
	}, {
		Name: "update builders configmap",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testBuilders.Get(),
			testApplicationBuilderReady.Get(),
			testFunctionBuilderReady.Get(),
		},
		ExpectUpdates: []runtime.Object{
			testBuilders.
				AddData("riff-application", testApplicationImage).
				AddData("riff-function", testFunctionImage).
				Get(),
		},
	}, {
		Name: "update builders configmap, error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("update", "ConfigMap"),
		},
		GivenObjects: []runtime.Object{
			testBuilders.Get(),
			testApplicationBuilderReady.Get(),
			testFunctionBuilderReady.Get(),
		},
		ShouldErr: true,
		ExpectUpdates: []runtime.Object{
			testBuilders.
				AddData("riff-application", testApplicationImage).
				AddData("riff-function", testFunctionImage).
				Get(),
		},
	}, {
		Name: "get builders configmap error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "ConfigMap"),
		},
		GivenObjects: []runtime.Object{
			testBuilders.Get(),
			testApplicationBuilder.Get(),
			testFunctionBuilder.Get(),
		},
		ShouldErr: true,
	}, {
		Name: "list builders error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("list", "ClusterBuilderList"),
		},
		GivenObjects: []runtime.Object{
			testBuilders.Get(),
			testApplicationBuilder.Get(),
			testFunctionBuilder.Get(),
		},
		ShouldErr: true,
	}}

	table.Test(t, scheme, func(t *testing.T, row *rtesting.Testcase, client client.Client, tracker tracker.Tracker, log logr.Logger) reconcile.Reconciler {
		return &build.ClusterBuilderReconciler{
			Client:    client,
			Log:       log,
			Namespace: testNamespace,
		}
	})
}
