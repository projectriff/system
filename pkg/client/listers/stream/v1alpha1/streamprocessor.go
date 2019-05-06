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
	v1alpha1 "github.com/projectriff/system/pkg/apis/stream/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// StreamProcessorLister helps list StreamProcessors.
type StreamProcessorLister interface {
	// List lists all StreamProcessors in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.StreamProcessor, err error)
	// StreamProcessors returns an object that can list and get StreamProcessors.
	StreamProcessors(namespace string) StreamProcessorNamespaceLister
	StreamProcessorListerExpansion
}

// streamProcessorLister implements the StreamProcessorLister interface.
type streamProcessorLister struct {
	indexer cache.Indexer
}

// NewStreamProcessorLister returns a new StreamProcessorLister.
func NewStreamProcessorLister(indexer cache.Indexer) StreamProcessorLister {
	return &streamProcessorLister{indexer: indexer}
}

// List lists all StreamProcessors in the indexer.
func (s *streamProcessorLister) List(selector labels.Selector) (ret []*v1alpha1.StreamProcessor, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.StreamProcessor))
	})
	return ret, err
}

// StreamProcessors returns an object that can list and get StreamProcessors.
func (s *streamProcessorLister) StreamProcessors(namespace string) StreamProcessorNamespaceLister {
	return streamProcessorNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// StreamProcessorNamespaceLister helps list and get StreamProcessors.
type StreamProcessorNamespaceLister interface {
	// List lists all StreamProcessors in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1alpha1.StreamProcessor, err error)
	// Get retrieves the StreamProcessor from the indexer for a given namespace and name.
	Get(name string) (*v1alpha1.StreamProcessor, error)
	StreamProcessorNamespaceListerExpansion
}

// streamProcessorNamespaceLister implements the StreamProcessorNamespaceLister
// interface.
type streamProcessorNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all StreamProcessors in the indexer for a given namespace.
func (s streamProcessorNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.StreamProcessor, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.StreamProcessor))
	})
	return ret, err
}

// Get retrieves the StreamProcessor from the indexer for a given namespace and name.
func (s streamProcessorNamespaceLister) Get(name string) (*v1alpha1.StreamProcessor, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("streamprocessor"), name)
	}
	return obj.(*v1alpha1.StreamProcessor), nil
}
