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

// BuildPlansGetter has a method to return a BuildPlanInterface.
// A group's client should implement this interface.
type BuildPlansGetter interface {
	BuildPlans(namespace string) BuildPlanInterface
}

// BuildPlanInterface has methods to work with BuildPlan resources.
type BuildPlanInterface interface {
	Create(*v1alpha1.BuildPlan) (*v1alpha1.BuildPlan, error)
	Update(*v1alpha1.BuildPlan) (*v1alpha1.BuildPlan, error)
	UpdateStatus(*v1alpha1.BuildPlan) (*v1alpha1.BuildPlan, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.BuildPlan, error)
	List(opts v1.ListOptions) (*v1alpha1.BuildPlanList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.BuildPlan, err error)
	BuildPlanExpansion
}

// buildPlans implements BuildPlanInterface
type buildPlans struct {
	client rest.Interface
	ns     string
}

// newBuildPlans returns a BuildPlans
func newBuildPlans(c *PlanV1alpha1Client, namespace string) *buildPlans {
	return &buildPlans{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the buildPlan, and returns the corresponding buildPlan object, and an error if there is any.
func (c *buildPlans) Get(name string, options v1.GetOptions) (result *v1alpha1.BuildPlan, err error) {
	result = &v1alpha1.BuildPlan{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("buildplans").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of BuildPlans that match those selectors.
func (c *buildPlans) List(opts v1.ListOptions) (result *v1alpha1.BuildPlanList, err error) {
	result = &v1alpha1.BuildPlanList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("buildplans").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested buildPlans.
func (c *buildPlans) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("buildplans").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a buildPlan and creates it.  Returns the server's representation of the buildPlan, and an error, if there is any.
func (c *buildPlans) Create(buildPlan *v1alpha1.BuildPlan) (result *v1alpha1.BuildPlan, err error) {
	result = &v1alpha1.BuildPlan{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("buildplans").
		Body(buildPlan).
		Do().
		Into(result)
	return
}

// Update takes the representation of a buildPlan and updates it. Returns the server's representation of the buildPlan, and an error, if there is any.
func (c *buildPlans) Update(buildPlan *v1alpha1.BuildPlan) (result *v1alpha1.BuildPlan, err error) {
	result = &v1alpha1.BuildPlan{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("buildplans").
		Name(buildPlan.Name).
		Body(buildPlan).
		Do().
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().

func (c *buildPlans) UpdateStatus(buildPlan *v1alpha1.BuildPlan) (result *v1alpha1.BuildPlan, err error) {
	result = &v1alpha1.BuildPlan{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("buildplans").
		Name(buildPlan.Name).
		SubResource("status").
		Body(buildPlan).
		Do().
		Into(result)
	return
}

// Delete takes name of the buildPlan and deletes it. Returns an error if one occurs.
func (c *buildPlans) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("buildplans").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *buildPlans) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("buildplans").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched buildPlan.
func (c *buildPlans) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.BuildPlan, err error) {
	result = &v1alpha1.BuildPlan{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("buildplans").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
