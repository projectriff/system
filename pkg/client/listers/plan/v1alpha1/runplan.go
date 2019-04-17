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

// RunPlanLister helps list RunPlans.
type RunPlanLister interface {
	// List lists all RunPlans in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.RunPlan, err error)
	// RunPlans returns an object that can list and get RunPlans.
	RunPlans(namespace string) RunPlanNamespaceLister
	RunPlanListerExpansion
}

// runPlanLister implements the RunPlanLister interface.
type runPlanLister struct {
	indexer cache.Indexer
}

// NewRunPlanLister returns a new RunPlanLister.
func NewRunPlanLister(indexer cache.Indexer) RunPlanLister {
	return &runPlanLister{indexer: indexer}
}

// List lists all RunPlans in the indexer.
func (s *runPlanLister) List(selector labels.Selector) (ret []*v1alpha1.RunPlan, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.RunPlan))
	})
	return ret, err
}

// RunPlans returns an object that can list and get RunPlans.
func (s *runPlanLister) RunPlans(namespace string) RunPlanNamespaceLister {
	return runPlanNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// RunPlanNamespaceLister helps list and get RunPlans.
type RunPlanNamespaceLister interface {
	// List lists all RunPlans in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1alpha1.RunPlan, err error)
	// Get retrieves the RunPlan from the indexer for a given namespace and name.
	Get(name string) (*v1alpha1.RunPlan, error)
	RunPlanNamespaceListerExpansion
}

// runPlanNamespaceLister implements the RunPlanNamespaceLister
// interface.
type runPlanNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all RunPlans in the indexer for a given namespace.
func (s runPlanNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.RunPlan, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.RunPlan))
	})
	return ret, err
}

// Get retrieves the RunPlan from the indexer for a given namespace and name.
func (s runPlanNamespaceLister) Get(name string) (*v1alpha1.RunPlan, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("runplan"), name)
	}
	return obj.(*v1alpha1.RunPlan), nil
}
