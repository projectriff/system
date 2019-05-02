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
	v1alpha1 "github.com/projectriff/system/pkg/apis/run/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// RequestProcessorLister helps list RequestProcessors.
type RequestProcessorLister interface {
	// List lists all RequestProcessors in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.RequestProcessor, err error)
	// RequestProcessors returns an object that can list and get RequestProcessors.
	RequestProcessors(namespace string) RequestProcessorNamespaceLister
	RequestProcessorListerExpansion
}

// requestProcessorLister implements the RequestProcessorLister interface.
type requestProcessorLister struct {
	indexer cache.Indexer
}

// NewRequestProcessorLister returns a new RequestProcessorLister.
func NewRequestProcessorLister(indexer cache.Indexer) RequestProcessorLister {
	return &requestProcessorLister{indexer: indexer}
}

// List lists all RequestProcessors in the indexer.
func (s *requestProcessorLister) List(selector labels.Selector) (ret []*v1alpha1.RequestProcessor, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.RequestProcessor))
	})
	return ret, err
}

// RequestProcessors returns an object that can list and get RequestProcessors.
func (s *requestProcessorLister) RequestProcessors(namespace string) RequestProcessorNamespaceLister {
	return requestProcessorNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// RequestProcessorNamespaceLister helps list and get RequestProcessors.
type RequestProcessorNamespaceLister interface {
	// List lists all RequestProcessors in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1alpha1.RequestProcessor, err error)
	// Get retrieves the RequestProcessor from the indexer for a given namespace and name.
	Get(name string) (*v1alpha1.RequestProcessor, error)
	RequestProcessorNamespaceListerExpansion
}

// requestProcessorNamespaceLister implements the RequestProcessorNamespaceLister
// interface.
type requestProcessorNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all RequestProcessors in the indexer for a given namespace.
func (s requestProcessorNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.RequestProcessor, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.RequestProcessor))
	})
	return ret, err
}

// Get retrieves the RequestProcessor from the indexer for a given namespace and name.
func (s requestProcessorNamespaceLister) Get(name string) (*v1alpha1.RequestProcessor, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("requestprocessor"), name)
	}
	return obj.(*v1alpha1.RequestProcessor), nil
}
