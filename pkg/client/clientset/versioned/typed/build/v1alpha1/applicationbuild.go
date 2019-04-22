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
	scheme "github.com/projectriff/system/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// ApplicationBuildsGetter has a method to return a ApplicationBuildInterface.
// A group's client should implement this interface.
type ApplicationBuildsGetter interface {
	ApplicationBuilds(namespace string) ApplicationBuildInterface
}

// ApplicationBuildInterface has methods to work with ApplicationBuild resources.
type ApplicationBuildInterface interface {
	Create(*v1alpha1.ApplicationBuild) (*v1alpha1.ApplicationBuild, error)
	Update(*v1alpha1.ApplicationBuild) (*v1alpha1.ApplicationBuild, error)
	UpdateStatus(*v1alpha1.ApplicationBuild) (*v1alpha1.ApplicationBuild, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.ApplicationBuild, error)
	List(opts v1.ListOptions) (*v1alpha1.ApplicationBuildList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.ApplicationBuild, err error)
	ApplicationBuildExpansion
}

// applicationBuilds implements ApplicationBuildInterface
type applicationBuilds struct {
	client rest.Interface
	ns     string
}

// newApplicationBuilds returns a ApplicationBuilds
func newApplicationBuilds(c *BuildV1alpha1Client, namespace string) *applicationBuilds {
	return &applicationBuilds{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the applicationBuild, and returns the corresponding applicationBuild object, and an error if there is any.
func (c *applicationBuilds) Get(name string, options v1.GetOptions) (result *v1alpha1.ApplicationBuild, err error) {
	result = &v1alpha1.ApplicationBuild{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("applicationbuilds").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ApplicationBuilds that match those selectors.
func (c *applicationBuilds) List(opts v1.ListOptions) (result *v1alpha1.ApplicationBuildList, err error) {
	result = &v1alpha1.ApplicationBuildList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("applicationbuilds").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested applicationBuilds.
func (c *applicationBuilds) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("applicationbuilds").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a applicationBuild and creates it.  Returns the server's representation of the applicationBuild, and an error, if there is any.
func (c *applicationBuilds) Create(applicationBuild *v1alpha1.ApplicationBuild) (result *v1alpha1.ApplicationBuild, err error) {
	result = &v1alpha1.ApplicationBuild{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("applicationbuilds").
		Body(applicationBuild).
		Do().
		Into(result)
	return
}

// Update takes the representation of a applicationBuild and updates it. Returns the server's representation of the applicationBuild, and an error, if there is any.
func (c *applicationBuilds) Update(applicationBuild *v1alpha1.ApplicationBuild) (result *v1alpha1.ApplicationBuild, err error) {
	result = &v1alpha1.ApplicationBuild{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("applicationbuilds").
		Name(applicationBuild.Name).
		Body(applicationBuild).
		Do().
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().

func (c *applicationBuilds) UpdateStatus(applicationBuild *v1alpha1.ApplicationBuild) (result *v1alpha1.ApplicationBuild, err error) {
	result = &v1alpha1.ApplicationBuild{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("applicationbuilds").
		Name(applicationBuild.Name).
		SubResource("status").
		Body(applicationBuild).
		Do().
		Into(result)
	return
}

// Delete takes name of the applicationBuild and deletes it. Returns an error if one occurs.
func (c *applicationBuilds) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("applicationbuilds").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *applicationBuilds) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("applicationbuilds").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched applicationBuild.
func (c *applicationBuilds) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.ApplicationBuild, err error) {
	result = &v1alpha1.ApplicationBuild{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("applicationbuilds").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
