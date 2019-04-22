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
	v1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// InternalBuildLister helps list InternalBuilds.
type InternalBuildLister interface {
	// List lists all InternalBuilds in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.InternalBuild, err error)
	// InternalBuilds returns an object that can list and get InternalBuilds.
	InternalBuilds(namespace string) InternalBuildNamespaceLister
	InternalBuildListerExpansion
}

// internalBuildLister implements the InternalBuildLister interface.
type internalBuildLister struct {
	indexer cache.Indexer
}

// NewInternalBuildLister returns a new InternalBuildLister.
func NewInternalBuildLister(indexer cache.Indexer) InternalBuildLister {
	return &internalBuildLister{indexer: indexer}
}

// List lists all InternalBuilds in the indexer.
func (s *internalBuildLister) List(selector labels.Selector) (ret []*v1alpha1.InternalBuild, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.InternalBuild))
	})
	return ret, err
}

// InternalBuilds returns an object that can list and get InternalBuilds.
func (s *internalBuildLister) InternalBuilds(namespace string) InternalBuildNamespaceLister {
	return internalBuildNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// InternalBuildNamespaceLister helps list and get InternalBuilds.
type InternalBuildNamespaceLister interface {
	// List lists all InternalBuilds in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1alpha1.InternalBuild, err error)
	// Get retrieves the InternalBuild from the indexer for a given namespace and name.
	Get(name string) (*v1alpha1.InternalBuild, error)
	InternalBuildNamespaceListerExpansion
}

// internalBuildNamespaceLister implements the InternalBuildNamespaceLister
// interface.
type internalBuildNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all InternalBuilds in the indexer for a given namespace.
func (s internalBuildNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.InternalBuild, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.InternalBuild))
	})
	return ret, err
}

// Get retrieves the InternalBuild from the indexer for a given namespace and name.
func (s internalBuildNamespaceLister) Get(name string) (*v1alpha1.InternalBuild, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("internalbuild"), name)
	}
	return obj.(*v1alpha1.InternalBuild), nil
}
