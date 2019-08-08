/*
 * Copyright 2019 The original author or authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package v1alpha1

import (
	time "time"

	knative_v1alpha1 "github.com/projectriff/system/pkg/apis/knative/v1alpha1"
	versioned "github.com/projectriff/system/pkg/client/clientset/versioned"
	internalinterfaces "github.com/projectriff/system/pkg/client/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/projectriff/system/pkg/client/listers/knative/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// ConfigurerInformer provides access to a shared informer and lister for
// Configurers.
type ConfigurerInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.ConfigurerLister
}

type configurerInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewConfigurerInformer constructs a new informer for Configurer type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewConfigurerInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredConfigurerInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredConfigurerInformer constructs a new informer for Configurer type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredConfigurerInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.KnativeV1alpha1().Configurers(namespace).List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.KnativeV1alpha1().Configurers(namespace).Watch(options)
			},
		},
		&knative_v1alpha1.Configurer{},
		resyncPeriod,
		indexers,
	)
}

func (f *configurerInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredConfigurerInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *configurerInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&knative_v1alpha1.Configurer{}, f.defaultInformer)
}

func (f *configurerInformer) Lister() v1alpha1.ConfigurerLister {
	return v1alpha1.NewConfigurerLister(f.Informer().GetIndexer())
}
