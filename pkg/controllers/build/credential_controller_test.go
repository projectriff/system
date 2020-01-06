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
	"github.com/projectriff/system/pkg/controllers/build"
	rtesting "github.com/projectriff/system/pkg/controllers/testing"
	"github.com/projectriff/system/pkg/controllers/testing/factories"
	"github.com/projectriff/system/pkg/tracker"
)

func TestCredentialsReconcile(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "riff-build"
	testKey := types.NamespacedName{Namespace: testNamespace, Name: testName}

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = buildv1alpha1.AddToScheme(scheme)

	testServiceAccount := factories.ServiceAccount().
		NamespaceName(testNamespace, testName)
	testCredential := factories.Secret().
		ObjectMeta(func(om factories.ObjectMeta) {
			om.AddLabel(buildv1alpha1.CredentialLabelKey, "docker-hub")
		}).
		NamespaceName(testNamespace, "my-credential")

	testApplication := factories.Application().
		NamespaceName(testNamespace, "my-application")
	testFunction := factories.Function().
		NamespaceName(testNamespace, "my-function")
	testContainer := factories.Container().
		NamespaceName(testNamespace, "my-container")

	table := rtesting.Table{{
		Name: "service account does not exist",
		Key:  testKey,
	}, {
		Name: "ignore deleted service account",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testServiceAccount.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.Deleted(1)
				}).
				Get(),
		},
	}, {
		Name: "ignore non-build service accounts",
		Key:  types.NamespacedName{Namespace: testNamespace, Name: "not-riff-build"},
		GivenObjects: []runtime.Object{
			testServiceAccount.
				NamespaceName(testNamespace, "not-riff-build").
				Get(),
		},
	}, {
		Name: "error fetching service account",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "ServiceAccount"),
		},
		GivenObjects: []runtime.Object{
			testServiceAccount.Get(),
		},
		ShouldErr: true,
	}, {
		Name: "create service account for application",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testApplication.Get(),
		},
		ExpectCreates: []runtime.Object{
			testServiceAccount.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddAnnotation("build.projectriff.io/credentials", "")
				}).
				Get(),
		},
	}, {
		Name: "create service account for application, listing fail",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("list", "ApplicationList"),
		},
		GivenObjects: []runtime.Object{
			testApplication.Get(),
		},
		ShouldErr: true,
	}, {
		Name: "create service account for function",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testFunction.Get(),
		},
		ExpectCreates: []runtime.Object{
			testServiceAccount.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddAnnotation("build.projectriff.io/credentials", "")
				}).
				Get(),
		},
	}, {
		Name: "create service account for function, listing fail",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("list", "FunctionList"),
		},
		GivenObjects: []runtime.Object{
			testFunction.Get(),
		},
		ShouldErr: true,
	}, {
		Name: "create service account for container",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testContainer.Get(),
		},
		ExpectCreates: []runtime.Object{
			testServiceAccount.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddAnnotation("build.projectriff.io/credentials", "")
				}).
				Get(),
		},
	}, {
		Name: "create service account for container, listing fail",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("list", "ContainerList"),
		},
		GivenObjects: []runtime.Object{
			testContainer.Get(),
		},
		ShouldErr: true,
	}, {
		Name: "create service account for credential",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testCredential.Get(),
		},
		ExpectCreates: []runtime.Object{
			testServiceAccount.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddAnnotation("build.projectriff.io/credentials", testCredential.Get().Name)
				}).
				Secrets(testCredential.Get().Name).
				Get(),
		},
	}, {
		Name: "create service account for credential, listing fail",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("list", "SecretList"),
		},
		GivenObjects: []runtime.Object{
			testCredential.Get(),
		},
		ShouldErr: true,
	}, {
		Name: "create service account, error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("create", "ServiceAccount"),
		},
		GivenObjects: []runtime.Object{
			testCredential.Get(),
		},
		ShouldErr: true,
		ExpectCreates: []runtime.Object{
			testServiceAccount.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddAnnotation("build.projectriff.io/credentials", testCredential.Get().Name)
				}).
				Secrets(testCredential.Get().Name).
				Get(),
		},
	}, {
		Name: "add credential to service account",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testServiceAccount.Get(),
			testCredential.Get(),
		},
		ExpectUpdates: []runtime.Object{
			testServiceAccount.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddAnnotation("build.projectriff.io/credentials", testCredential.Get().Name)
				}).
				Secrets(testCredential.Get().Name).
				Get(),
		},
	}, {
		Name: "add credentials to service account",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testServiceAccount.Get(),
			testCredential.
				NamespaceName(testNamespace, "cred-1").
				Get(),
			testCredential.
				NamespaceName(testNamespace, "cred-2").
				Get(),
		},
		ExpectUpdates: []runtime.Object{
			testServiceAccount.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddAnnotation("build.projectriff.io/credentials", "cred-1,cred-2")
				}).
				Secrets("cred-1", "cred-2").
				Get(),
		},
	}, {
		Name: "add credential to service account, list credentials error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("list", "SecretList"),
		},
		GivenObjects: []runtime.Object{
			testServiceAccount.Get(),
			testCredential.Get(),
		},
		ShouldErr: true,
	}, {
		Name: "add credential to service account, update error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("update", "ServiceAccount"),
		},
		GivenObjects: []runtime.Object{
			testServiceAccount.Get(),
			testCredential.Get(),
		},
		ShouldErr: true,
		ExpectUpdates: []runtime.Object{
			testServiceAccount.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddAnnotation("build.projectriff.io/credentials", testCredential.Get().Name)
				}).
				Secrets(testCredential.Get().Name).
				Get(),
		},
	}, {
		Name: "add credentials to service account, preserving non-credential bound secrets",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testServiceAccount.
				Secrets("keep-me").
				Get(),
			testCredential.
				NamespaceName(testNamespace, "cred-1").
				Get(),
			testCredential.
				NamespaceName(testNamespace, "cred-2").
				Get(),
		},
		ExpectUpdates: []runtime.Object{
			testServiceAccount.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddAnnotation("build.projectriff.io/credentials", "cred-1,cred-2")
				}).
				Secrets("keep-me", "cred-1", "cred-2").
				Get(),
		},
	}, {
		Name: "ignore non-credential secrets for service account",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testServiceAccount.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddAnnotation("build.projectriff.io/credentials", "")
				}).
				Secrets().
				Get(),
			factories.Secret().
				NamespaceName(testNamespace, "not-a-credential").
				Get(),
		},
	}, {
		Name: "remove credential from service account",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testServiceAccount.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddAnnotation("build.projectriff.io/credentials", "cred-1,cred-2")
				}).
				Secrets("cred-1", "cred-2").
				Get(),
		},
		ExpectUpdates: []runtime.Object{
			testServiceAccount.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddAnnotation("build.projectriff.io/credentials", "")
				}).
				Secrets().
				Get(),
		},
	}, {
		Name: "remove credential from service account, preserving non-credential bound secrets",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			testServiceAccount.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddAnnotation("build.projectriff.io/credentials", "cred-1,cred-2")
				}).
				Secrets("keep-me", "cred-1", "cred-2").
				Get(),
		},
		ExpectUpdates: []runtime.Object{
			testServiceAccount.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddAnnotation("build.projectriff.io/credentials", "")
				}).
				Secrets("keep-me").
				Get(),
		},
	}}

	table.Test(t, scheme, func(t *testing.T, row *rtesting.Testcase, client client.Client, tracker tracker.Tracker, log logr.Logger) reconcile.Reconciler {
		return &build.CredentialReconciler{
			Client: client,
			Log:    log,
		}
	})
}
