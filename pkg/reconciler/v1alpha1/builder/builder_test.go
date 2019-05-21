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

package builder

import (
	"testing"

	knbuildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	fakeknbuildclientset "github.com/knative/build/pkg/client/clientset/versioned/fake"
	knbuildinformers "github.com/knative/build/pkg/client/informers/externalversions"
	"github.com/knative/pkg/controller"
	"github.com/projectriff/system/pkg/reconciler"
	. "github.com/projectriff/system/pkg/reconciler/v1alpha1/testing"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
)

func TestReconcile(t *testing.T) {
	appBuilder := "cloudfoundry/cnb:bionic"
	fnBuilder := "projectriff/builder"

	table := TableTest{{
		Name: "bad workqueue key",
		Key:  "too/many/parts",
	}, {
		Name:    "create configmap",
		Key:     "riff-system/builders",
		Objects: []runtime.Object{},
		WantCreates: []metav1.Object{
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "riff-system",
					Name:      "builders",
				},
				Data: map[string]string{},
			},
		},
		WantEvents: []string{
			`Normal Created Created ConfigMap "builders"`,
		},
	}, {
		Name: "create builders configmap",
		Key:  "riff-system/builders",
		Objects: []runtime.Object{
			&knbuildv1alpha1.ClusterBuildTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "riff-function",
				},
				Spec: knbuildv1alpha1.BuildTemplateSpec{
					Parameters: []knbuildv1alpha1.ParameterSpec{
						{Name: "BUILDER_IMAGE", Default: &fnBuilder},
					},
				},
			},
		},
		WantCreates: []metav1.Object{
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "riff-system",
					Name:      "builders",
				},
				Data: map[string]string{
					"riff-function": fnBuilder,
				},
			},
		},
		WantEvents: []string{
			`Normal Created Created ConfigMap "builders"`,
		},
	}, {
		Name: "add builder",
		Key:  "riff-system/builders",
		Objects: []runtime.Object{
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "riff-system",
					Name:      "builders",
				},
				Data: map[string]string{
					"riff-function": fnBuilder,
				},
			},
			&knbuildv1alpha1.ClusterBuildTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "riff-application",
				},
				Spec: knbuildv1alpha1.BuildTemplateSpec{
					Parameters: []knbuildv1alpha1.ParameterSpec{
						{Name: "BUILDER_IMAGE", Default: &appBuilder},
					},
				},
			},
			&knbuildv1alpha1.ClusterBuildTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "riff-function",
				},
				Spec: knbuildv1alpha1.BuildTemplateSpec{
					Parameters: []knbuildv1alpha1.ParameterSpec{
						{Name: "BUILDER_IMAGE", Default: &fnBuilder},
					},
				},
			},
		},
		WantUpdates: []UpdateActionImpl{
			{
				Object: &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "riff-system",
						Name:      "builders",
					},
					Data: map[string]string{
						"riff-application": appBuilder,
						"riff-function":    fnBuilder,
					},
				},
			},
		},
		WantEvents: []string{
			`Normal Updated Reconciled ConfigMap "builders"`,
		},
	}, {
		Name: "remove builder",
		Key:  "riff-system/builders",
		Objects: []runtime.Object{
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "riff-system",
					Name:      "builders",
				},
				Data: map[string]string{
					"riff-application": appBuilder,
					"riff-function":    fnBuilder,
				},
			},
			&knbuildv1alpha1.ClusterBuildTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "riff-application",
				},
				Spec: knbuildv1alpha1.BuildTemplateSpec{
					Parameters: []knbuildv1alpha1.ParameterSpec{
						{Name: "BUILDER_IMAGE", Default: &appBuilder},
					},
				},
			},
		},
		WantUpdates: []UpdateActionImpl{
			{
				Object: &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "riff-system",
						Name:      "builders",
					},
					Data: map[string]string{
						"riff-application": appBuilder,
					},
				},
			},
		},
		WantEvents: []string{
			`Normal Updated Reconciled ConfigMap "builders"`,
		},
	}}

	defer ClearAllLoggers()
	table.Test(t, MakeFactory(func(listers *Listers, opt reconciler.Options) controller.Reconciler {
		return &Reconciler{
			Base:                         reconciler.NewBase(opt, controllerAgentName),
			configmapLister:              listers.GetConfigMapLister(),
			knclusterbuildtemplateLister: listers.GetKnClusterBuildTemplateLister(),
		}
	}))
}

func TestNew(t *testing.T) {
	defer ClearAllLoggers()
	kubeClient := fakekubeclientset.NewSimpleClientset()
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)
	knbuildClient := fakeknbuildclientset.NewSimpleClientset()
	knbuildInformer := knbuildinformers.NewSharedInformerFactory(knbuildClient, 0)

	configmapInformer := kubeInformer.Core().V1().ConfigMaps()
	knclusterbuildtemplateInformer := knbuildInformer.Build().V1alpha1().ClusterBuildTemplates()

	c := NewController(reconciler.Options{
		KubeClientSet:    kubeClient,
		KnBuildClientSet: knbuildClient,
		Logger:           TestLogger(t),
	}, configmapInformer, knclusterbuildtemplateInformer)

	if c == nil {
		t.Fatal("Expected NewController to return a non-nil value")
	}
}
