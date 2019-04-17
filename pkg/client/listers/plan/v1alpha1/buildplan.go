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
	v1alpha1 "github.com/projectriff/system/pkg/apis/plan/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// BuildPlanLister helps list BuildPlans.
type BuildPlanLister interface {
	// List lists all BuildPlans in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.BuildPlan, err error)
	// BuildPlans returns an object that can list and get BuildPlans.
	BuildPlans(namespace string) BuildPlanNamespaceLister
	BuildPlanListerExpansion
}

// buildPlanLister implements the BuildPlanLister interface.
type buildPlanLister struct {
	indexer cache.Indexer
}

// NewBuildPlanLister returns a new BuildPlanLister.
func NewBuildPlanLister(indexer cache.Indexer) BuildPlanLister {
	return &buildPlanLister{indexer: indexer}
}

// List lists all BuildPlans in the indexer.
func (s *buildPlanLister) List(selector labels.Selector) (ret []*v1alpha1.BuildPlan, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.BuildPlan))
	})
	return ret, err
}

// BuildPlans returns an object that can list and get BuildPlans.
func (s *buildPlanLister) BuildPlans(namespace string) BuildPlanNamespaceLister {
	return buildPlanNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// BuildPlanNamespaceLister helps list and get BuildPlans.
type BuildPlanNamespaceLister interface {
	// List lists all BuildPlans in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1alpha1.BuildPlan, err error)
	// Get retrieves the BuildPlan from the indexer for a given namespace and name.
	Get(name string) (*v1alpha1.BuildPlan, error)
	BuildPlanNamespaceListerExpansion
}

// buildPlanNamespaceLister implements the BuildPlanNamespaceLister
// interface.
type buildPlanNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all BuildPlans in the indexer for a given namespace.
func (s buildPlanNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.BuildPlan, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.BuildPlan))
	})
	return ret, err
}

// Get retrieves the BuildPlan from the indexer for a given namespace and name.
func (s buildPlanNamespaceLister) Get(name string) (*v1alpha1.BuildPlan, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("buildplan"), name)
	}
	return obj.(*v1alpha1.BuildPlan), nil
}
