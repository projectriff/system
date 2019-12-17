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
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	"github.com/projectriff/system/pkg/refs"
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

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = kpackbuildv1alpha1.AddToScheme(scheme)
	_ = buildv1alpha1.AddToScheme(scheme)

	table := rtesting.Table{{
		Name: "application does not exist",
		Key:  testKey,
	}, {
		Name: "create kpack image, default image",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "riff-build",
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"default-image-prefix": testImagePrefix,
				},
			},
			&buildv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
				},
				Spec: buildv1alpha1.ApplicationSpec{
					Source: &buildv1alpha1.Source{
						Git: &buildv1alpha1.Git{
							URL:      testGitUrl,
							Revision: testGitRevision,
						},
					},
				},
			},
		},
		ExpectCreates: []runtime.Object{
			&kpackbuildv1alpha1.Image{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: fmt.Sprintf("%s-application-", testName),
					Namespace:    testNamespace,
					Labels: map[string]string{
						buildv1alpha1.ApplicationLabelKey: testName,
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         buildv1alpha1.GroupVersion.String(),
							Kind:               "Application",
							Name:               testName,
							Controller:         rtesting.BoolPtr(true),
							BlockOwnerDeletion: rtesting.BoolPtr(true),
						},
					},
				},
				Spec: kpackbuildv1alpha1.ImageSpec{
					Tag: fmt.Sprintf("%s/%s", testImagePrefix, testName),
					Builder: kpackbuildv1alpha1.ImageBuilder{
						TypeMeta: metav1.TypeMeta{
							Kind: "ClusterBuilder",
						},
						Name: "riff-application",
					},
					ServiceAccount: "riff-build",
					Source: kpackbuildv1alpha1.SourceConfig{
						Git: &kpackbuildv1alpha1.Git{
							URL:      testGitUrl,
							Revision: testGitRevision,
						},
					},
				},
			},
		},
		ExpectStatusUpdates: []runtime.Object{
			&buildv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
				},
				Status: buildv1alpha1.ApplicationStatus{
					Status: apis.Status{
						Conditions: []apis.Condition{
							{
								Type:   buildv1alpha1.ApplicationConditionImageResolved,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
								Status: corev1.ConditionUnknown,
							},
							{
								Type:   buildv1alpha1.ApplicationConditionReady,
								Status: corev1.ConditionUnknown,
							},
						},
					},
					BuildStatus: buildv1alpha1.BuildStatus{
						KpackImageRef: refs.NewTypedLocalObjectReferenceForObject(&kpackbuildv1alpha1.Image{
							ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-application-001", testName)},
						}, scheme),
						TargetImage: fmt.Sprintf("%s/%s", testImagePrefix, testName),
					},
				},
			},
		},
	}, {
		Name: "kpack image ready",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "riff-build",
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"default-image-prefix": testImagePrefix,
				},
			},
			&buildv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
				},
				Spec: buildv1alpha1.ApplicationSpec{
					Source: &buildv1alpha1.Source{
						Git: &buildv1alpha1.Git{
							URL:      testGitUrl,
							Revision: testGitRevision,
						},
					},
				},
			},
			&kpackbuildv1alpha1.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: testNamespace,
					Labels: map[string]string{
						buildv1alpha1.ApplicationLabelKey: testName,
					},
					Generation: 1,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         buildv1alpha1.GroupVersion.String(),
							Kind:               "Application",
							Name:               testName,
							Controller:         rtesting.BoolPtr(true),
							BlockOwnerDeletion: rtesting.BoolPtr(true),
						},
					},
				},
				Spec: kpackbuildv1alpha1.ImageSpec{
					Tag: fmt.Sprintf("%s/%s", testImagePrefix, testName),
					Builder: kpackbuildv1alpha1.ImageBuilder{
						TypeMeta: metav1.TypeMeta{
							Kind: "ClusterBuilder",
						},
						Name: "riff-application",
					},
					ServiceAccount: "riff-build",
					Source: kpackbuildv1alpha1.SourceConfig{
						Git: &kpackbuildv1alpha1.Git{
							URL:      testGitUrl,
							Revision: testGitRevision,
						},
					},
				},
				Status: kpackbuildv1alpha1.ImageStatus{
					LatestImage:  fmt.Sprintf("%s/%s@sha256:%s", testImagePrefix, testName, testSha256),
					BuildCounter: 1,
					Status: apis.Status{
						ObservedGeneration: 1,
						Conditions: []apis.Condition{
							{
								Type:   apis.ConditionReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
		},
		ExpectStatusUpdates: []runtime.Object{
			&buildv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
				},
				Status: buildv1alpha1.ApplicationStatus{
					Status: apis.Status{
						Conditions: []apis.Condition{
							{
								Type:   buildv1alpha1.ApplicationConditionImageResolved,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   buildv1alpha1.ApplicationConditionKpackImageReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   buildv1alpha1.ApplicationConditionReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
					BuildStatus: buildv1alpha1.BuildStatus{
						KpackImageRef: refs.NewTypedLocalObjectReferenceForObject(&kpackbuildv1alpha1.Image{
							ObjectMeta: metav1.ObjectMeta{Name: "test"},
						}, scheme),
						TargetImage: fmt.Sprintf("%s/%s", testImagePrefix, testName),
						LatestImage: fmt.Sprintf("%s/%s@sha256:%s", testImagePrefix, testName, testSha256),
					},
				},
			},
		},
	}, {
		Name: "kpack image not-ready",
		Key:  testKey,
		GivenObjects: []runtime.Object{
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "riff-build",
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"default-image-prefix": testImagePrefix,
				},
			},
			&buildv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
				},
				Spec: buildv1alpha1.ApplicationSpec{
					Source: &buildv1alpha1.Source{
						Git: &buildv1alpha1.Git{
							URL:      testGitUrl,
							Revision: testGitRevision,
						},
					},
				},
			},
			&kpackbuildv1alpha1.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: testNamespace,
					Labels: map[string]string{
						buildv1alpha1.ApplicationLabelKey: testName,
					},
					Generation: 1,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         buildv1alpha1.GroupVersion.String(),
							Kind:               "Application",
							Name:               testName,
							Controller:         rtesting.BoolPtr(true),
							BlockOwnerDeletion: rtesting.BoolPtr(true),
						},
					},
				},
				Spec: kpackbuildv1alpha1.ImageSpec{
					Tag: fmt.Sprintf("%s/%s", testImagePrefix, testName),
					Builder: kpackbuildv1alpha1.ImageBuilder{
						TypeMeta: metav1.TypeMeta{
							Kind: "ClusterBuilder",
						},
						Name: "riff-application",
					},
					ServiceAccount: "riff-build",
					Source: kpackbuildv1alpha1.SourceConfig{
						Git: &kpackbuildv1alpha1.Git{
							URL:      testGitUrl,
							Revision: testGitRevision,
						},
					},
				},
				Status: kpackbuildv1alpha1.ImageStatus{
					LatestImage:  fmt.Sprintf("%s/%s@sha256:%s", testImagePrefix, testName, testSha256),
					BuildCounter: 1,
					Status: apis.Status{
						ObservedGeneration: 1,
						Conditions: []apis.Condition{
							{
								Type:    apis.ConditionReady,
								Status:  corev1.ConditionFalse,
								Reason:  testConditionReason,
								Message: testConditionMessage,
							},
						},
					},
				},
			},
		},
		ExpectStatusUpdates: []runtime.Object{
			&buildv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
				},
				Status: buildv1alpha1.ApplicationStatus{
					Status: apis.Status{
						Conditions: []apis.Condition{
							{
								Type:   buildv1alpha1.ApplicationConditionImageResolved,
								Status: corev1.ConditionTrue,
							},
							{
								Type:    buildv1alpha1.ApplicationConditionKpackImageReady,
								Status:  corev1.ConditionFalse,
								Reason:  testConditionReason,
								Message: testConditionMessage,
							},
							{
								Type:    buildv1alpha1.ApplicationConditionReady,
								Status:  corev1.ConditionFalse,
								Reason:  testConditionReason,
								Message: testConditionMessage,
							},
						},
					},
					BuildStatus: buildv1alpha1.BuildStatus{
						KpackImageRef: refs.NewTypedLocalObjectReferenceForObject(&kpackbuildv1alpha1.Image{
							ObjectMeta: metav1.ObjectMeta{Name: "test"},
						}, scheme),
						TargetImage: fmt.Sprintf("%s/%s", testImagePrefix, testName),
						LatestImage: fmt.Sprintf("%s/%s@sha256:%s", testImagePrefix, testName, testSha256),
					},
				},
			},
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
