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

package knativehandler

import (
	"testing"

	"github.com/knative/pkg/controller"
	fakeknservingclientset "github.com/knative/serving/pkg/client/clientset/versioned/fake"
	knservinginformers "github.com/knative/serving/pkg/client/informers/externalversions"
	fakeprojectriffclientset "github.com/projectriff/system/pkg/client/clientset/versioned/fake"
	projectriffinformers "github.com/projectriff/system/pkg/client/informers/externalversions"
	"github.com/projectriff/system/pkg/reconciler"
	rtesting "github.com/projectriff/system/pkg/reconciler/testing"
	. "github.com/projectriff/system/pkg/reconciler/v1alpha1/testing"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
)

func TestReconcile(t *testing.T) {
	table := TableTest{{
		Name: "bad workqueue key",
		Key:  "too/many/parts",
	}, {
		Name: "key not found",
		Key:  "foo/not-found",
	}}

	defer ClearAllLoggers()
	table.Test(t, MakeFactory(func(listers *Listers, opt reconciler.Options) controller.Reconciler {
		return &Reconciler{
			Base:                  reconciler.NewBase(opt, controllerAgentName),
			handlerLister:         listers.GetKnativeHandlerLister(),
			knconfigurationLister: listers.GetKnConfigurationLister(),
			knrouteLister:         listers.GetKnRouteLister(),
			applicationLister:     listers.GetApplicationLister(),
			containerLister:       listers.GetContainerLister(),
			functionLister:        listers.GetFunctionLister(),

			tracker: &rtesting.NullTracker{},
		}
	}))
}

func TestNew(t *testing.T) {
	defer ClearAllLoggers()
	kubeClient := fakekubeclientset.NewSimpleClientset()
	projectriffClient := fakeprojectriffclientset.NewSimpleClientset()
	projectriffInformer := projectriffinformers.NewSharedInformerFactory(projectriffClient, 0)
	knservingClient := fakeknservingclientset.NewSimpleClientset()
	knservingInformer := knservinginformers.NewSharedInformerFactory(knservingClient, 0)

	handlerInformer := projectriffInformer.Knative().V1alpha1().Handlers()
	knconfigurationInformer := knservingInformer.Serving().V1alpha1().Configurations()
	knrouteInformer := knservingInformer.Serving().V1alpha1().Routes()
	applicationInformer := projectriffInformer.Build().V1alpha1().Applications()
	containerInformer := projectriffInformer.Build().V1alpha1().Containers()
	functionInformer := projectriffInformer.Build().V1alpha1().Functions()

	c := NewController(reconciler.Options{
		KubeClientSet:        kubeClient,
		ProjectriffClientSet: projectriffClient,
		KnServingClientSet:   knservingClient,
		Logger:               TestLogger(t),
	}, handlerInformer, knconfigurationInformer, knrouteInformer, applicationInformer, containerInformer, functionInformer)

	if c == nil {
		t.Fatal("Expected NewController to return a non-nil value")
	}
}
