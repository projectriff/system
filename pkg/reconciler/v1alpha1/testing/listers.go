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

package testing

import (
	knbuildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	fakeknbuildclientset "github.com/knative/build/pkg/client/clientset/versioned/fake"
	knbuildv1alpha1listers "github.com/knative/build/pkg/client/listers/build/v1alpha1"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	fakeknservingclientset "github.com/knative/serving/pkg/client/clientset/versioned/fake"
	knservingv1alpha1listers "github.com/knative/serving/pkg/client/listers/serving/v1alpha1"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	streamsv1alpha1 "github.com/projectriff/system/pkg/apis/streams/v1alpha1"
	runv1alpha1 "github.com/projectriff/system/pkg/apis/run/v1alpha1"
	fakeprojectriffclientset "github.com/projectriff/system/pkg/client/clientset/versioned/fake"
	buildv1alpha1listers "github.com/projectriff/system/pkg/client/listers/build/v1alpha1"
	runv1alpha1listers "github.com/projectriff/system/pkg/client/listers/run/v1alpha1"
	streamsv1alpha1listers "github.com/projectriff/system/pkg/client/listers/streams/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/testing"
	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
	corev1listers "k8s.io/client-go/listers/core/v1"
	appsv1listers "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
)

var clientSetSchemes = []func(*runtime.Scheme) error{
	fakekubeclientset.AddToScheme,
	fakeprojectriffclientset.AddToScheme,
	fakeknbuildclientset.AddToScheme,
	fakeknservingclientset.AddToScheme,
}

type Listers struct {
	sorter testing.ObjectSorter
}

func NewListers(objs []runtime.Object) Listers {
	scheme := runtime.NewScheme()

	for _, addTo := range clientSetSchemes {
		addTo(scheme)
	}

	ls := Listers{
		sorter: testing.NewObjectSorter(scheme),
	}

	ls.sorter.AddObjects(objs...)

	return ls
}

func (l *Listers) indexerFor(obj runtime.Object) cache.Indexer {
	return l.sorter.IndexerForObjectType(obj)
}

func (l *Listers) GetProjectriffObjects() []runtime.Object {
	return l.sorter.ObjectsForSchemeFunc(fakeprojectriffclientset.AddToScheme)
}

func (l *Listers) GetKnBuildObjects() []runtime.Object {
	return l.sorter.ObjectsForSchemeFunc(fakeknbuildclientset.AddToScheme)
}

func (l *Listers) GetKnServingObjects() []runtime.Object {
	return l.sorter.ObjectsForSchemeFunc(fakeknservingclientset.AddToScheme)
}

func (l *Listers) GetKubeObjects() []runtime.Object {
	return l.sorter.ObjectsForSchemeFunc(fakekubeclientset.AddToScheme)
}

func (l *Listers) GetApplicationLister() buildv1alpha1listers.ApplicationLister {
	return buildv1alpha1listers.NewApplicationLister(l.indexerFor(&buildv1alpha1.Application{}))
}

func (l *Listers) GetFunctionLister() buildv1alpha1listers.FunctionLister {
	return buildv1alpha1listers.NewFunctionLister(l.indexerFor(&buildv1alpha1.Function{}))
}

func (l *Listers) GetRequestProcessorLister() runv1alpha1listers.RequestProcessorLister {
	return runv1alpha1listers.NewRequestProcessorLister(l.indexerFor(&runv1alpha1.RequestProcessor{}))
}

func (l *Listers) GetStreamLister() streamsv1alpha1listers.StreamLister {
	return streamsv1alpha1listers.NewStreamLister(l.indexerFor(&streamsv1alpha1.Stream{}))
}

func (l *Listers) GetStreamProcessorLister() streamsv1alpha1listers.StreamProcessorLister {
	return streamsv1alpha1listers.NewStreamProcessorLister(l.indexerFor(&streamsv1alpha1.StreamProcessor{}))
}

func (l *Listers) GetKnBuildLister() knbuildv1alpha1listers.BuildLister {
	return knbuildv1alpha1listers.NewBuildLister(l.indexerFor(&knbuildv1alpha1.Build{}))
}

func (l *Listers) GetKnConfigurationLister() knservingv1alpha1listers.ConfigurationLister {
	return knservingv1alpha1listers.NewConfigurationLister(l.indexerFor(&knservingv1alpha1.Configuration{}))
}

func (l *Listers) GetKnRouteLister() knservingv1alpha1listers.RouteLister {
	return knservingv1alpha1listers.NewRouteLister(l.indexerFor(&knservingv1alpha1.Route{}))
}

func (l *Listers) GetPersistentVolumeClaimLister() corev1listers.PersistentVolumeClaimLister {
	return corev1listers.NewPersistentVolumeClaimLister(l.indexerFor(&corev1.PersistentVolumeClaim{}))
}

func (l *Listers) GetConfigMapLister() corev1listers.ConfigMapLister {
	return corev1listers.NewConfigMapLister(l.indexerFor(&corev1.ConfigMap{}))
}

func (l *Listers) GetDeploymentLister() appsv1listers.DeploymentLister {
	return appsv1listers.NewDeploymentLister(l.indexerFor(&appsv1.Deployment{}))
}
