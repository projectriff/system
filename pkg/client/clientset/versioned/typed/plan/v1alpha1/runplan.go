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
	scheme "github.com/projectriff/system/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// RunPlansGetter has a method to return a RunPlanInterface.
// A group's client should implement this interface.
type RunPlansGetter interface {
	RunPlans(namespace string) RunPlanInterface
}

// RunPlanInterface has methods to work with RunPlan resources.
type RunPlanInterface interface {
	Create(*v1alpha1.RunPlan) (*v1alpha1.RunPlan, error)
	Update(*v1alpha1.RunPlan) (*v1alpha1.RunPlan, error)
	UpdateStatus(*v1alpha1.RunPlan) (*v1alpha1.RunPlan, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.RunPlan, error)
	List(opts v1.ListOptions) (*v1alpha1.RunPlanList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.RunPlan, err error)
	RunPlanExpansion
}

// runPlans implements RunPlanInterface
type runPlans struct {
	client rest.Interface
	ns     string
}

// newRunPlans returns a RunPlans
func newRunPlans(c *PlanV1alpha1Client, namespace string) *runPlans {
	return &runPlans{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the runPlan, and returns the corresponding runPlan object, and an error if there is any.
func (c *runPlans) Get(name string, options v1.GetOptions) (result *v1alpha1.RunPlan, err error) {
	result = &v1alpha1.RunPlan{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("runplans").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of RunPlans that match those selectors.
func (c *runPlans) List(opts v1.ListOptions) (result *v1alpha1.RunPlanList, err error) {
	result = &v1alpha1.RunPlanList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("runplans").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested runPlans.
func (c *runPlans) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("runplans").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a runPlan and creates it.  Returns the server's representation of the runPlan, and an error, if there is any.
func (c *runPlans) Create(runPlan *v1alpha1.RunPlan) (result *v1alpha1.RunPlan, err error) {
	result = &v1alpha1.RunPlan{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("runplans").
		Body(runPlan).
		Do().
		Into(result)
	return
}

// Update takes the representation of a runPlan and updates it. Returns the server's representation of the runPlan, and an error, if there is any.
func (c *runPlans) Update(runPlan *v1alpha1.RunPlan) (result *v1alpha1.RunPlan, err error) {
	result = &v1alpha1.RunPlan{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("runplans").
		Name(runPlan.Name).
		Body(runPlan).
		Do().
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().

func (c *runPlans) UpdateStatus(runPlan *v1alpha1.RunPlan) (result *v1alpha1.RunPlan, err error) {
	result = &v1alpha1.RunPlan{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("runplans").
		Name(runPlan.Name).
		SubResource("status").
		Body(runPlan).
		Do().
		Into(result)
	return
}

// Delete takes name of the runPlan and deletes it. Returns an error if one occurs.
func (c *runPlans) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("runplans").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *runPlans) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("runplans").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched runPlan.
func (c *runPlans) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.RunPlan, err error) {
	result = &v1alpha1.RunPlan{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("runplans").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
