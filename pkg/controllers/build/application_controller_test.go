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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/projectriff/system/pkg/apis"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	kpackbuildv1alpha1 "github.com/projectriff/system/pkg/apis/thirdparty/kpack/build/v1alpha1"
	"github.com/projectriff/system/pkg/controllers/build"
	rtesting "github.com/projectriff/system/pkg/controllers/testing"
	"github.com/projectriff/system/pkg/controllers/testing/factories"
	"github.com/projectriff/system/pkg/tracker"
)

func TestApplicationReconcile(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-application"
	testKey := types.NamespacedName{Namespace: testNamespace, Name: testName}
	testImagePrefix := "example.com/repo"
	testGitUrl := "git@example.com:repo.git"
	testGitRevision := "master"
	testSha256 := "cf8b4c69d5460f88530e1c80b8856a70801f31c50b191c8413043ba9b160a43e"
	testConditionReason := "TestReason"
	testConditionMessage := "meaningful, yet concise"
	testLabelKey := "test-label-key"
	testLabelValue := "test-label-value"
	testBuildCacheName := "test-build-cache-000"

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = kpackbuildv1alpha1.AddToScheme(scheme)
	_ = buildv1alpha1.AddToScheme(scheme)

	appMinimal := factories.Application().
		NamespaceName(testNamespace, testName)
	appValid := appMinimal.
		Image("%s/%s", testImagePrefix, testName).
		SourceGit(testGitUrl, testGitRevision)

	kpackImageCreate := factories.KpackImage().
		ObjectMeta(func(om factories.ObjectMeta) {
			om.Namespace(testNamespace).
				GenerateName("%s-application-", testName).
				AddLabel(buildv1alpha1.ApplicationLabelKey, testName).
				ControlledBy(appMinimal.Get(), scheme)
		}).
		Tag("%s/%s", testImagePrefix, testName).
		ApplicationBuilder().
		SourceGit(testGitUrl, testGitRevision)
	kpackImageGiven := kpackImageCreate.
		ObjectMeta(func(om factories.ObjectMeta) {
			om.
				Name("%s-application-001", testName).
				Generation(1)
		}).
		StatusObservedGeneration(1)

	cmImagePrefix := factories.ConfigMap().
		NamespaceName(testNamespace, "riff-build").
		AddData("default-image-prefix", "")

	table := rtesting.Table{{
		Name: "application does not exist",
		Key:  testKey,
	}, {
		Name: "ignore deleted application",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			appValid.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.Deleted(1)
				}).
				Get(),
		},
	}, {
		Name: "application get error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "Application"),
		},
		ShouldErr: true,
	}, {
		Name: "create kpack image",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			appValid.Get(),
		},
		ExpectCreates: []runtime.Object{
			kpackImageCreate.Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionUnknown,
					},
				).
				StatusKpackImageRef("%s-application-001", testName).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
	}, {
		Name: "create kpack image, build cache",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			appValid.
				BuildCache("1Gi").
				Get(),
		},
		ExpectCreates: []runtime.Object{
			kpackImageCreate.
				BuildCache("1Gi").
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.FunctionConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.FunctionConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   buildv1alpha1.FunctionConditionReady,
						Status: corev1.ConditionUnknown,
					},
				).
				StatusKpackImageRef("%s-application-001", testName).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
	}, {
		Name: "create kpack image, propagating labels",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			appValid.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddLabel(testLabelKey, testLabelValue)
				}).
				Get(),
		},
		ExpectCreates: []runtime.Object{
			kpackImageCreate.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddLabel(testLabelKey, testLabelValue)
				}).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionUnknown,
					},
				).
				StatusKpackImageRef("%s-application-001", testName).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
	}, {
		Name: "default image",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			cmImagePrefix.
				AddData("default-image-prefix", testImagePrefix).
				Get(),
			appMinimal.
				SourceGit(testGitUrl, testGitRevision).
				Get(),
		},
		ExpectCreates: []runtime.Object{
			kpackImageCreate.Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionUnknown,
					},
				).
				StatusKpackImageRef("%s-application-001", testName).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
	}, {
		Name: "default image, missing",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			appMinimal.
				SourceGit(testGitUrl, testGitRevision).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:    buildv1alpha1.ApplicationConditionImageResolved,
						Reason:  "DefaultImagePrefixMissing",
						Message: "missing default image prefix",
						Status:  corev1.ConditionFalse,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:    buildv1alpha1.ApplicationConditionReady,
						Reason:  "DefaultImagePrefixMissing",
						Message: "missing default image prefix",
						Status:  corev1.ConditionFalse,
					},
				).
				Get(),
		},
		ShouldErr: true,
	}, {
		Name: "default image, undefined",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			cmImagePrefix.Get(),
			appMinimal.
				SourceGit(testGitUrl, testGitRevision).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:    buildv1alpha1.ApplicationConditionImageResolved,
						Reason:  "DefaultImagePrefixMissing",
						Message: "missing default image prefix",
						Status:  corev1.ConditionFalse,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:    buildv1alpha1.ApplicationConditionReady,
						Reason:  "DefaultImagePrefixMissing",
						Message: "missing default image prefix",
						Status:  corev1.ConditionFalse,
					},
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
			appMinimal.
				SourceGit(testGitUrl, testGitRevision).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:    buildv1alpha1.ApplicationConditionImageResolved,
						Reason:  "ImageInvalid",
						Message: "inducing failure for get ConfigMap",
						Status:  corev1.ConditionFalse,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:    buildv1alpha1.ApplicationConditionReady,
						Reason:  "ImageInvalid",
						Message: "inducing failure for get ConfigMap",
						Status:  corev1.ConditionFalse,
					},
				).
				Get(),
		},
		ShouldErr: true,
	}, {
		Name: "kpack image ready",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			appValid.Get(),
			kpackImageGiven.
				StatusReady().
				StatusLatestImage("%s/%s@sha256:%s", testImagePrefix, testName, testSha256).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionTrue,
					},
				).
				StatusKpackImageRef(kpackImageGiven.Get().Name).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				StatusLatestImage("%s/%s@sha256:%s", testImagePrefix, testName, testSha256).
				Get(),
		},
	}, {
		Name: "kpack image ready, build cache",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			appValid.Get(),
			kpackImageGiven.
				StatusReady().
				StatusBuildCacheName(testBuildCacheName).
				StatusLatestImage("%s/%s@sha256:%s", testImagePrefix, testName, testSha256).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionTrue,
					},
				).
				StatusKpackImageRef(kpackImageGiven.Get().Name).
				StatusBuildCacheRef(testBuildCacheName).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				StatusLatestImage("%s/%s@sha256:%s", testImagePrefix, testName, testSha256).
				Get(),
		},
	}, {
		Name: "kpack image not-ready",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			appValid.Get(),
			kpackImageGiven.
				StatusConditions(
					apis.Condition{
						Type:    apis.ConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  testConditionReason,
						Message: testConditionMessage,
					},
				).
				StatusLatestImage("%s/%s@sha256:%s", testImagePrefix, testName, testSha256).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:    buildv1alpha1.ApplicationConditionKpackImageReady,
						Status:  corev1.ConditionFalse,
						Reason:  testConditionReason,
						Message: testConditionMessage,
					},
					apis.Condition{
						Type:    buildv1alpha1.ApplicationConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  testConditionReason,
						Message: testConditionMessage,
					},
				).
				StatusKpackImageRef(kpackImageGiven.Get().Name).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				StatusLatestImage("%s/%s@sha256:%s", testImagePrefix, testName, testSha256).
				Get(),
		},
	}, {
		Name: "kpack image create error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("create", "Image"),
		},
		GivenObjects: []runtime.Object{
			appValid.Get(),
		},
		ShouldErr: true,
		ExpectCreates: []runtime.Object{
			kpackImageCreate.Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appValid.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionUnknown,
					},
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
	}, {
		Name: "kpack image update, spec",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			appValid.Get(),
			kpackImageGiven.
				SourceGit(testGitUrl, "bogus").
				Get(),
		},
		ExpectUpdates: []runtime.Object{
			kpackImageGiven.Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appValid.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionUnknown,
					},
				).
				StatusKpackImageRef(kpackImageGiven.Get().Name).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
	}, {
		Name: "kpack image update, labels",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			appValid.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddLabel(testLabelKey, testLabelValue)
				}).
				Get(),
			kpackImageGiven.Get(),
		},
		ExpectUpdates: []runtime.Object{
			kpackImageGiven.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddLabel(testLabelKey, testLabelValue)
				}).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appValid.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionUnknown,
					},
				).
				StatusKpackImageRef(kpackImageGiven.Get().Name).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
	}, {
		Name: "kpack image update, fails",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("update", "Image"),
		},
		GivenObjects: []runtime.Object{
			appValid.Get(),
			kpackImageGiven.
				SourceGit(testGitUrl, "bogus").
				Get(),
		},
		ShouldErr: true,
		ExpectUpdates: []runtime.Object{
			kpackImageGiven.Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appValid.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionUnknown,
					},
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
	}, {
		Name: "kpack image list error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("list", "ImageList"),
		},
		GivenObjects: []runtime.Object{
			appValid.Get(),
		},
		ShouldErr: true,
		ExpectStatusUpdates: []runtime.Object{
			appValid.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionUnknown,
					},
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
	}, {
		Name: "application status update error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("update", "Application"),
		},
		GivenObjects: []runtime.Object{
			appValid.Get(),
		},
		ExpectCreates: []runtime.Object{
			kpackImageCreate.Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionUnknown,
					},
				).
				StatusKpackImageRef("%s-application-001", testName).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
		ShouldErr: true,
	}, {
		Name: "delete extra kpack image",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			appValid.Get(),
			kpackImageGiven.
				NamespaceName(testNamespace, "extra1").
				Get(),
			kpackImageGiven.
				NamespaceName(testNamespace, "extra2").
				Get(),
		},
		ExpectDeletes: []rtesting.DeleteRef{
			{Group: "build.pivotal.io", Kind: "Image", Namespace: testNamespace, Name: "extra1"},
			{Group: "build.pivotal.io", Kind: "Image", Namespace: testNamespace, Name: "extra2"},
		},
		ExpectCreates: []runtime.Object{
			kpackImageCreate.Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionUnknown,
					},
				).
				StatusKpackImageRef("%s-application-001", testName).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
	}, {
		Name: "delete extra kpack image, fails",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("delete", "Image"),
		},
		GivenObjects: []runtime.Object{
			appValid.Get(),
			kpackImageGiven.
				NamespaceName(testNamespace, "extra1").
				Get(),
			kpackImageGiven.
				NamespaceName(testNamespace, "extra2").
				Get(),
		},
		ShouldErr: true,
		ExpectDeletes: []rtesting.DeleteRef{
			{Group: "build.pivotal.io", Kind: "Image", Namespace: testNamespace, Name: "extra1"},
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionUnknown,
					},
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
	}, {
		Name: "local build",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			appMinimal.
				Image("%s/%s", testImagePrefix, testName).
				Get(),
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionTrue,
					},
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				// TODO resolve to a digest
				StatusLatestImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
	}, {
		Name: "local build, removes existing build",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			appMinimal.
				Image("%s/%s", testImagePrefix, testName).
				Get(),
			kpackImageGiven.Get(),
		},
		ExpectDeletes: []rtesting.DeleteRef{
			{Group: "build.pivotal.io", Kind: "Image", Namespace: kpackImageGiven.Get().Namespace, Name: kpackImageGiven.Get().Name},
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionTrue,
					},
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				// TODO resolve to a digest
				StatusLatestImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
	}, {
		Name: "local build, removes existing build, error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("delete", "Image"),
		},
		GivenObjects: []runtime.Object{
			appMinimal.
				Image("%s/%s", testImagePrefix, testName).
				Get(),
			kpackImageGiven.Get(),
		},
		ShouldErr: true,
		ExpectDeletes: []rtesting.DeleteRef{
			{Group: "build.pivotal.io", Kind: "Image", Namespace: kpackImageGiven.Get().Namespace, Name: kpackImageGiven.Get().Name},
		},
		ExpectStatusUpdates: []runtime.Object{
			appMinimal.
				StatusConditions(
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionImageResolved,
						Status: corev1.ConditionTrue,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
						Status: corev1.ConditionUnknown,
					},
					apis.Condition{
						Type:   buildv1alpha1.ApplicationConditionReady,
						Status: corev1.ConditionUnknown,
					},
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				Get(),
		},
	}}

	table.Test(t, scheme, func(t *testing.T, row *rtesting.Testcase, client client.Client, tracker tracker.Tracker, log logr.Logger) reconcile.Reconciler {
		return &build.ApplicationReconciler{
			Client: client,
			Scheme: scheme,
			Log:    log,
		}
	})
}
