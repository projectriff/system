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

package streamingprocessor

import (
	"testing"

	fakekedaclientset "github.com/kedacore/keda/pkg/client/clientset/versioned/fake"
	kedainformers "github.com/kedacore/keda/pkg/client/informers/externalversions"
	"github.com/knative/pkg/controller"
	fakeprojectriffclientset "github.com/projectriff/system/pkg/client/clientset/versioned/fake"
	projectriffinformers "github.com/projectriff/system/pkg/client/informers/externalversions"
	"github.com/projectriff/system/pkg/reconciler"
	rtesting "github.com/projectriff/system/pkg/reconciler/testing"
	. "github.com/projectriff/system/pkg/reconciler/v1alpha1/testing"
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
	}}

	defer ClearAllLoggers()
	table.Test(t, MakeFactory(func(listers *Listers, opt reconciler.Options) controller.Reconciler {
		return &Reconciler{
			Base:             reconciler.NewBase(opt, controllerAgentName),
			processorLister:  listers.GetStreamingProcessorLister(),
			functionLister:   listers.GetFunctionLister(),
			streamLister:     listers.GetStreamingStreamLister(),
			deploymentLister: listers.GetDeploymentLister(),
			scaledObjectLister:listers.GetScaledObjectLister(),

			tracker: &rtesting.NullTracker{},
		}
	}))
}

func TestNew(t *testing.T) {
	defer ClearAllLoggers()
	kubeClient := fakekubeclientset.NewSimpleClientset()
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)
	projectriffClient := fakeprojectriffclientset.NewSimpleClientset()
	projectriffInformer := projectriffinformers.NewSharedInformerFactory(projectriffClient, 0)
	kedaClient := fakekedaclientset.NewSimpleClientset()
	kedaInformer := kedainformers.NewSharedInformerFactory(kedaClient, 0)

	processorInformer := projectriffInformer.Streaming().V1alpha1().Processors()
	streamInformer := projectriffInformer.Streaming().V1alpha1().Streams()
	functionInformer := projectriffInformer.Build().V1alpha1().Functions()
	deploymentInformer := kubeInformer.Apps().V1().Deployments()
	scaledObjectInformer := kedaInformer.Keda().V1alpha1().ScaledObjects()

	c := NewController(reconciler.Options{
		KubeClientSet:        kubeClient,
		ProjectriffClientSet: projectriffClient,
		KedaClientSet:        kedaClient,
		Logger:               TestLogger(t),
	}, processorInformer, functionInformer, streamInformer, deploymentInformer, scaledObjectInformer)

	if c == nil {
		t.Fatal("Expected NewController to return a non-nil value")
	}
}
