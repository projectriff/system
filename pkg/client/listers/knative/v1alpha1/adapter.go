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
	v1alpha1 "github.com/projectriff/system/pkg/apis/knative/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// AdapterLister helps list Adapters.
type AdapterLister interface {
	// List lists all Adapters in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.Adapter, err error)
	// Adapters returns an object that can list and get Adapters.
	Adapters(namespace string) AdapterNamespaceLister
	AdapterListerExpansion
}

// adapterLister implements the AdapterLister interface.
type adapterLister struct {
	indexer cache.Indexer
}

// NewAdapterLister returns a new AdapterLister.
func NewAdapterLister(indexer cache.Indexer) AdapterLister {
	return &adapterLister{indexer: indexer}
}

// List lists all Adapters in the indexer.
func (s *adapterLister) List(selector labels.Selector) (ret []*v1alpha1.Adapter, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Adapter))
	})
	return ret, err
}

// Adapters returns an object that can list and get Adapters.
func (s *adapterLister) Adapters(namespace string) AdapterNamespaceLister {
	return adapterNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// AdapterNamespaceLister helps list and get Adapters.
type AdapterNamespaceLister interface {
	// List lists all Adapters in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1alpha1.Adapter, err error)
	// Get retrieves the Adapter from the indexer for a given namespace and name.
	Get(name string) (*v1alpha1.Adapter, error)
	AdapterNamespaceListerExpansion
}

// adapterNamespaceLister implements the AdapterNamespaceLister
// interface.
type adapterNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all Adapters in the indexer for a given namespace.
func (s adapterNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.Adapter, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Adapter))
	})
	return ret, err
}

// Get retrieves the Adapter from the indexer for a given namespace and name.
func (s adapterNamespaceLister) Get(name string) (*v1alpha1.Adapter, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("adapter"), name)
	}
	return obj.(*v1alpha1.Adapter), nil
}
