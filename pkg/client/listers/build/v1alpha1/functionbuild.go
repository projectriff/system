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

// FunctionBuildLister helps list FunctionBuilds.
type FunctionBuildLister interface {
	// List lists all FunctionBuilds in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.FunctionBuild, err error)
	// FunctionBuilds returns an object that can list and get FunctionBuilds.
	FunctionBuilds(namespace string) FunctionBuildNamespaceLister
	FunctionBuildListerExpansion
}

// functionBuildLister implements the FunctionBuildLister interface.
type functionBuildLister struct {
	indexer cache.Indexer
}

// NewFunctionBuildLister returns a new FunctionBuildLister.
func NewFunctionBuildLister(indexer cache.Indexer) FunctionBuildLister {
	return &functionBuildLister{indexer: indexer}
}

// List lists all FunctionBuilds in the indexer.
func (s *functionBuildLister) List(selector labels.Selector) (ret []*v1alpha1.FunctionBuild, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.FunctionBuild))
	})
	return ret, err
}

// FunctionBuilds returns an object that can list and get FunctionBuilds.
func (s *functionBuildLister) FunctionBuilds(namespace string) FunctionBuildNamespaceLister {
	return functionBuildNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// FunctionBuildNamespaceLister helps list and get FunctionBuilds.
type FunctionBuildNamespaceLister interface {
	// List lists all FunctionBuilds in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1alpha1.FunctionBuild, err error)
	// Get retrieves the FunctionBuild from the indexer for a given namespace and name.
	Get(name string) (*v1alpha1.FunctionBuild, error)
	FunctionBuildNamespaceListerExpansion
}

// functionBuildNamespaceLister implements the FunctionBuildNamespaceLister
// interface.
type functionBuildNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all FunctionBuilds in the indexer for a given namespace.
func (s functionBuildNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.FunctionBuild, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.FunctionBuild))
	})
	return ret, err
}

// Get retrieves the FunctionBuild from the indexer for a given namespace and name.
func (s functionBuildNamespaceLister) Get(name string) (*v1alpha1.FunctionBuild, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("functionbuild"), name)
	}
	return obj.(*v1alpha1.FunctionBuild), nil
}
