/*
Copyright 2018 The Knative Authors

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

package credential

import (
	"testing"

	"github.com/knative/pkg/controller"
	"github.com/projectriff/system/pkg/apis/build"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	fakeprojectriffclientset "github.com/projectriff/system/pkg/client/clientset/versioned/fake"
	projectriffinformers "github.com/projectriff/system/pkg/client/informers/externalversions"
	"github.com/projectriff/system/pkg/reconciler"
	. "github.com/projectriff/system/pkg/reconciler/v1alpha1/testing"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
)

func TestReconcile(t *testing.T) {
	table := TableTest{{
		Name: "bad workqueue key",
		Key:  "too/many/parts",
	}, {
		Name: "key not found",
		Key:  "foo/not-found",
	}, {
		Name: "skips creating service account by default",
		Key:  "default/riff-build",
	}, {
		Name: "create service account with credential",
		Key:  "default/riff-build",
		Objects: []runtime.Object{
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-secret",
					Labels: map[string]string{
						build.CredentialLabelKey: "",
					},
				},
			},
		},
		WantCreates: []metav1.Object{
			&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "riff-build",
					Annotations: map[string]string{
						build.CredentialsAnnotationKey: "my-secret",
					},
				},
				Secrets: []corev1.ObjectReference{
					{Name: "my-secret"},
				},
			},
		},
		WantEvents: []string{
			`Normal Created Created ServiceAccount "riff-build"`,
		},
	}, {
		Name: "create service account with application",
		Key:  "default/riff-build",
		Objects: []runtime.Object{
			&buildv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-application",
				},
			},
		},
		WantCreates: []metav1.Object{
			&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "riff-build",
					Annotations: map[string]string{
						build.CredentialsAnnotationKey: "",
					},
				},
				Secrets: []corev1.ObjectReference{},
			},
		},
		WantEvents: []string{
			`Normal Created Created ServiceAccount "riff-build"`,
		},
	}, {
		Name: "create service account with function",
		Key:  "default/riff-build",
		Objects: []runtime.Object{
			&buildv1alpha1.Function{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-function",
				},
			},
		},
		WantCreates: []metav1.Object{
			&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "riff-build",
					Annotations: map[string]string{
						build.CredentialsAnnotationKey: "",
					},
				},
				Secrets: []corev1.ObjectReference{},
			},
		},
		WantEvents: []string{
			`Normal Created Created ServiceAccount "riff-build"`,
		},
	}, {
		Name: "create service account ignoring non-credential secrets",
		Key:  "default/riff-build",
		Objects: []runtime.Object{
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-secret",
				},
			},
		},
	}, {
		Name: "add credential",
		Key:  "default/riff-build",
		Objects: []runtime.Object{
			&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "riff-build",
				},
				Secrets: []corev1.ObjectReference{
					{Name: "riff-build-token-5p282"},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-secret",
					Labels: map[string]string{
						build.CredentialLabelKey: "",
					},
				},
			},
		},
		WantUpdates: []UpdateActionImpl{
			{
				Object: &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "riff-build",
						Annotations: map[string]string{
							build.CredentialsAnnotationKey: "my-secret",
						},
					},
					Secrets: []corev1.ObjectReference{
						{Name: "riff-build-token-5p282"},
						{Name: "my-secret"},
					},
				},
			},
		},
		WantEvents: []string{
			`Normal Updated Reconciled ServiceAccount credentials "riff-build"`,
		},
	}, {
		Name: "add additional credential",
		Key:  "default/riff-build",
		Objects: []runtime.Object{
			&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "riff-build",
					Annotations: map[string]string{
						build.CredentialsAnnotationKey: "my-secret",
					},
				},
				Secrets: []corev1.ObjectReference{
					{Name: "riff-build-token-5p282"},
					{Name: "my-secret"},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-secret",
					Labels: map[string]string{
						build.CredentialLabelKey: "",
					},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "other-secret",
					Labels: map[string]string{
						build.CredentialLabelKey: "",
					},
				},
			},
		},
		WantUpdates: []UpdateActionImpl{
			{
				Object: &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "riff-build",
						Annotations: map[string]string{
							build.CredentialsAnnotationKey: "my-secret,other-secret",
						},
					},
					Secrets: []corev1.ObjectReference{
						{Name: "riff-build-token-5p282"},
						{Name: "my-secret"},
						{Name: "other-secret"},
					},
				},
			},
		},
		WantEvents: []string{
			`Normal Updated Reconciled ServiceAccount credentials "riff-build"`,
		},
	}, {
		Name: "remove credential",
		Key:  "default/riff-build",
		Objects: []runtime.Object{
			&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "riff-build",
					Annotations: map[string]string{
						build.CredentialsAnnotationKey: "my-secret,other-secret",
					},
				},
				Secrets: []corev1.ObjectReference{
					{Name: "riff-build-token-5p282"},
					{Name: "my-secret"},
					{Name: "other-secret"},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "other-secret",
					Labels: map[string]string{
						build.CredentialLabelKey: "",
					},
				},
			},
		},
		WantUpdates: []UpdateActionImpl{
			{
				Object: &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "riff-build",
						Annotations: map[string]string{
							build.CredentialsAnnotationKey: "other-secret",
						},
					},
					Secrets: []corev1.ObjectReference{
						{Name: "riff-build-token-5p282"},
						{Name: "other-secret"},
					},
				},
			},
		},
		WantEvents: []string{
			`Normal Updated Reconciled ServiceAccount credentials "riff-build"`,
		},
	}}

	defer ClearAllLoggers()
	table.Test(t, MakeFactory(func(listers *Listers, opt reconciler.Options) controller.Reconciler {
		return &Reconciler{
			Base:                 reconciler.NewBase(opt, controllerAgentName),
			serviceAccountLister: listers.GetServiceAccountLister(),
			secretLister:         listers.GetSecretLister(),
			applicationLister:    listers.GetApplicationLister(),
			functionLister:       listers.GetFunctionLister(),
		}
	}))
}

func TestNew(t *testing.T) {
	defer ClearAllLoggers()
	kubeClient := fakekubeclientset.NewSimpleClientset()
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)
	projectriffClient := fakeprojectriffclientset.NewSimpleClientset()
	projectriffInformer := projectriffinformers.NewSharedInformerFactory(projectriffClient, 0)

	secretInformer := kubeInformer.Core().V1().Secrets()
	serviceAccountInformer := kubeInformer.Core().V1().ServiceAccounts()

	applicationInformer := projectriffInformer.Build().V1alpha1().Applications()
	functionInformer := projectriffInformer.Build().V1alpha1().Functions()

	c := NewController(reconciler.Options{
		KubeClientSet: kubeClient,
		Logger:        TestLogger(t),
	}, secretInformer, serviceAccountInformer, applicationInformer, functionInformer)

	if c == nil {
		t.Fatal("Expected NewController to return a non-nil value")
	}
}
