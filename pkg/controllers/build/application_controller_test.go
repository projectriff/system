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

	"github.com/projectriff/system/pkg/apis"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	kpackbuildv1alpha1 "github.com/projectriff/system/pkg/apis/thirdparty/kpack/build/v1alpha1"
	"github.com/projectriff/system/pkg/controllers/build"
	rtesting "github.com/projectriff/system/pkg/controllers/testing"
	"github.com/projectriff/system/pkg/controllers/testing/factories"
	"github.com/projectriff/system/pkg/tracker"
)

func TestApplicationReconciler(t *testing.T) {
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

	applicationConditionImageResolved := factories.Condition().Type(buildv1alpha1.ApplicationConditionImageResolved)
	applicationConditionKpackImageReady := factories.Condition().Type(buildv1alpha1.ApplicationConditionKpackImageReady)
	applicationConditionReady := factories.Condition().Type(buildv1alpha1.ApplicationConditionReady)

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
				ControlledBy(appMinimal, scheme)
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
		GivenObjects: []rtesting.Factory{
			appValid.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.Deleted(1)
				}),
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
		GivenObjects: []rtesting.Factory{
			appValid,
		},
		ExpectCreates: []rtesting.Factory{
			kpackImageCreate,
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.Unknown(),
				).
				StatusKpackImageRef("%s-application-001", testName).
				StatusTargetImage("%s/%s", testImagePrefix, testName),
		},
	}, {
		Name: "create kpack image, build cache",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			appValid.
				BuildCache("1Gi"),
		},
		ExpectCreates: []rtesting.Factory{
			kpackImageCreate.
				BuildCache("1Gi"),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.Unknown(),
				).
				StatusKpackImageRef("%s-application-001", testName).
				StatusTargetImage("%s/%s", testImagePrefix, testName),
		},
	}, {
		Name: "create kpack image, propagating labels",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			appValid.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddLabel(testLabelKey, testLabelValue)
				}),
		},
		ExpectCreates: []rtesting.Factory{
			kpackImageCreate.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddLabel(testLabelKey, testLabelValue)
				}),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.Unknown(),
				).
				StatusKpackImageRef("%s-application-001", testName).
				StatusTargetImage("%s/%s", testImagePrefix, testName),
		},
	}, {
		Name: "default image",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			cmImagePrefix.
				AddData("default-image-prefix", testImagePrefix),
			appMinimal.
				SourceGit(testGitUrl, testGitRevision),
		},
		ExpectCreates: []rtesting.Factory{
			kpackImageCreate,
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.Unknown(),
				).
				StatusKpackImageRef("%s-application-001", testName).
				StatusTargetImage("%s/%s", testImagePrefix, testName),
		},
	}, {
		Name: "default image, missing",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			appMinimal.
				SourceGit(testGitUrl, testGitRevision),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.False().Reason("DefaultImagePrefixMissing", "missing default image prefix"),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.False().Reason("DefaultImagePrefixMissing", "missing default image prefix"),
				),
		},
		ShouldErr: true,
	}, {
		Name: "default image, undefined",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			cmImagePrefix,
			appMinimal.
				SourceGit(testGitUrl, testGitRevision),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.False().Reason("DefaultImagePrefixMissing", "missing default image prefix"),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.False().Reason("DefaultImagePrefixMissing", "missing default image prefix"),
				),
		},
		ShouldErr: true,
	}, {
		Name: "default image, error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "ConfigMap"),
		},
		GivenObjects: []rtesting.Factory{
			cmImagePrefix,
			appMinimal.
				SourceGit(testGitUrl, testGitRevision),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.False().Reason("ImageInvalid", "inducing failure for get ConfigMap"),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.False().Reason("ImageInvalid", "inducing failure for get ConfigMap"),
				),
		},
		ShouldErr: true,
	}, {
		Name: "kpack image ready",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			appValid,
			kpackImageGiven.
				StatusReady().
				StatusLatestImage("%s/%s@sha256:%s", testImagePrefix, testName, testSha256),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.True(),
					applicationConditionReady.True(),
				).
				StatusKpackImageRef(kpackImageGiven.Get().GetName()).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				StatusLatestImage("%s/%s@sha256:%s", testImagePrefix, testName, testSha256),
		},
	}, {
		Name: "kpack image ready, build cache",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			appValid,
			kpackImageGiven.
				StatusReady().
				StatusBuildCacheName(testBuildCacheName).
				StatusLatestImage("%s/%s@sha256:%s", testImagePrefix, testName, testSha256),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.True(),
					applicationConditionReady.True(),
				).
				StatusKpackImageRef(kpackImageGiven.Get().GetName()).
				StatusBuildCacheRef(testBuildCacheName).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				StatusLatestImage("%s/%s@sha256:%s", testImagePrefix, testName, testSha256),
		},
	}, {
		Name: "kpack image not-ready",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			appValid,
			kpackImageGiven.
				StatusConditions(
					factories.Condition().Type(apis.ConditionReady).False().Reason(testConditionReason, testConditionMessage),
				).
				StatusLatestImage("%s/%s@sha256:%s", testImagePrefix, testName, testSha256),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.False().Reason(testConditionReason, testConditionMessage),
					applicationConditionReady.False().Reason(testConditionReason, testConditionMessage),
				).
				StatusKpackImageRef(kpackImageGiven.Get().GetName()).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				StatusLatestImage("%s/%s@sha256:%s", testImagePrefix, testName, testSha256),
		},
	}, {
		Name: "kpack image create error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("create", "Image"),
		},
		GivenObjects: []rtesting.Factory{
			appValid,
		},
		ShouldErr: true,
		ExpectCreates: []rtesting.Factory{
			kpackImageCreate,
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appValid.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.Unknown(),
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName),
		},
	}, {
		Name: "kpack image update, spec",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			appValid,
			kpackImageGiven.
				SourceGit(testGitUrl, "bogus"),
		},
		ExpectUpdates: []rtesting.Factory{
			kpackImageGiven,
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appValid.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.Unknown(),
				).
				StatusKpackImageRef(kpackImageGiven.Get().GetName()).
				StatusTargetImage("%s/%s", testImagePrefix, testName),
		},
	}, {
		Name: "kpack image update, labels",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			appValid.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddLabel(testLabelKey, testLabelValue)
				}),
			kpackImageGiven,
		},
		ExpectUpdates: []rtesting.Factory{
			kpackImageGiven.
				ObjectMeta(func(om factories.ObjectMeta) {
					om.AddLabel(testLabelKey, testLabelValue)
				}),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appValid.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.Unknown(),
				).
				StatusKpackImageRef(kpackImageGiven.Get().GetName()).
				StatusTargetImage("%s/%s", testImagePrefix, testName),
		},
	}, {
		Name: "kpack image update, fails",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("update", "Image"),
		},
		GivenObjects: []rtesting.Factory{
			appValid,
			kpackImageGiven.
				SourceGit(testGitUrl, "bogus"),
		},
		ShouldErr: true,
		ExpectUpdates: []rtesting.Factory{
			kpackImageGiven,
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appValid.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.Unknown(),
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName),
		},
	}, {
		Name: "kpack image list error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("list", "ImageList"),
		},
		GivenObjects: []rtesting.Factory{
			appValid,
		},
		ShouldErr: true,
		ExpectStatusUpdates: []rtesting.Factory{
			appValid.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.Unknown(),
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName),
		},
	}, {
		Name: "application status update error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("update", "Application"),
		},
		GivenObjects: []rtesting.Factory{
			appValid,
		},
		ExpectCreates: []rtesting.Factory{
			kpackImageCreate,
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.Unknown(),
				).
				StatusKpackImageRef("%s-application-001", testName).
				StatusTargetImage("%s/%s", testImagePrefix, testName),
		},
		ShouldErr: true,
	}, {
		Name: "delete extra kpack image",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			appValid,
			kpackImageGiven.
				NamespaceName(testNamespace, "extra1"),
			kpackImageGiven.
				NamespaceName(testNamespace, "extra2"),
		},
		ExpectDeletes: []rtesting.DeleteRef{
			{Group: "build.pivotal.io", Kind: "Image", Namespace: testNamespace, Name: "extra1"},
			{Group: "build.pivotal.io", Kind: "Image", Namespace: testNamespace, Name: "extra2"},
		},
		ExpectCreates: []rtesting.Factory{
			kpackImageCreate,
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.Unknown(),
				).
				StatusKpackImageRef("%s-application-001", testName).
				StatusTargetImage("%s/%s", testImagePrefix, testName),
		},
	}, {
		Name: "delete extra kpack image, fails",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("delete", "Image"),
		},
		GivenObjects: []rtesting.Factory{
			appValid,
			kpackImageGiven.
				NamespaceName(testNamespace, "extra1"),
			kpackImageGiven.
				NamespaceName(testNamespace, "extra2"),
		},
		ShouldErr: true,
		ExpectDeletes: []rtesting.DeleteRef{
			{Group: "build.pivotal.io", Kind: "Image", Namespace: testNamespace, Name: "extra1"},
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.Unknown(),
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName),
		},
	}, {
		Name: "local build",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			appMinimal.
				Image("%s/%s", testImagePrefix, testName),
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.True(),
					applicationConditionReady.True(),
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				// TODO resolve to a digest
				StatusLatestImage("%s/%s", testImagePrefix, testName),
		},
	}, {
		Name: "local build, removes existing build",
		Key:  testKey,
		GivenObjects: []rtesting.Factory{
			appMinimal.
				Image("%s/%s", testImagePrefix, testName),
			kpackImageGiven,
		},
		ExpectDeletes: []rtesting.DeleteRef{
			{Group: "build.pivotal.io", Kind: "Image", Namespace: kpackImageGiven.Get().GetNamespace(), Name: kpackImageGiven.Get().GetName()},
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.True(),
					applicationConditionReady.True(),
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName).
				// TODO resolve to a digest
				StatusLatestImage("%s/%s", testImagePrefix, testName),
		},
	}, {
		Name: "local build, removes existing build, error",
		Key:  testKey,
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("delete", "Image"),
		},
		GivenObjects: []rtesting.Factory{
			appMinimal.
				Image("%s/%s", testImagePrefix, testName),
			kpackImageGiven,
		},
		ShouldErr: true,
		ExpectDeletes: []rtesting.DeleteRef{
			{Group: "build.pivotal.io", Kind: "Image", Namespace: kpackImageGiven.Get().GetNamespace(), Name: kpackImageGiven.Get().GetName()},
		},
		ExpectStatusUpdates: []rtesting.Factory{
			appMinimal.
				StatusConditions(
					applicationConditionImageResolved.True(),
					applicationConditionKpackImageReady.Unknown(),
					applicationConditionReady.Unknown(),
				).
				StatusTargetImage("%s/%s", testImagePrefix, testName),
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
