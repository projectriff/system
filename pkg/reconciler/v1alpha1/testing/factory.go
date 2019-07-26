/*
Copyright 2018 The Knative Authors.

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

package testing

import (
	"testing"

	fakekedaclientset "github.com/kedacore/keda/pkg/client/clientset/versioned/fake"
	fakeknbuildclientset "github.com/knative/build/pkg/client/clientset/versioned/fake"
	"github.com/knative/pkg/controller"
	fakeprojectriffclientset "github.com/projectriff/system/pkg/client/clientset/versioned/fake"
	"github.com/projectriff/system/pkg/reconciler"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
)

const (
	// maxEventBufferSize is the estimated max number of event notifications that
	// can be buffered during reconciliation.
	maxEventBufferSize = 10
)

// Ctor functions create a k8s controller with given params.
type Ctor func(*Listers, reconciler.Options) controller.Reconciler

// MakeFactory creates a reconciler factory with fake clients and controller created by `ctor`.
func MakeFactory(ctor Ctor) Factory {
	return func(t *testing.T, r *TableRow) (controller.Reconciler, ActionRecorderList, EventList) {
		ls := NewListers(r.Objects)

		kubeClient := fakekubeclientset.NewSimpleClientset(ls.GetKubeObjects()...)
		projectriffClient := fakeprojectriffclientset.NewSimpleClientset(ls.GetProjectriffObjects()...)
		knbuildClient := fakeknbuildclientset.NewSimpleClientset(ls.GetKnBuildObjects()...)
		kedaClient := fakekedaclientset.NewSimpleClientset(ls.GetKedaObjects()...)
		eventRecorder := record.NewFakeRecorder(maxEventBufferSize)

		PrependGenerateNameReactor(&projectriffClient.Fake)

		// Set up our Controller from the fakes.
		c := ctor(&ls, reconciler.Options{
			KubeClientSet:        kubeClient,
			ProjectriffClientSet: projectriffClient,
			KnBuildClientSet:     knbuildClient,
			KedaClientSet:        kedaClient,
			Recorder:             eventRecorder,
			Logger:               TestLogger(t),
		})

		for _, reactor := range r.WithReactors {
			kubeClient.PrependReactor("*", "*", reactor)
			projectriffClient.PrependReactor("*", "*", reactor)
			knbuildClient.PrependReactor("*", "*", reactor)
			kedaClient.PrependReactor("*", "*", reactor)
		}

		// Validate all Create operations through the projectriff client.
		projectriffClient.PrependReactor("create", "*", ValidateCreates)
		projectriffClient.PrependReactor("update", "*", ValidateUpdates)

		actionRecorderList := ActionRecorderList{projectriffClient, kubeClient, knbuildClient}
		eventList := EventList{Recorder: eventRecorder}

		return c, actionRecorderList, eventList
	}
}
