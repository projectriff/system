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

// InternalBuildsGetter has a method to return a InternalBuildInterface.
// A group's client should implement this interface.
type InternalBuildsGetter interface {
	InternalBuilds(namespace string) InternalBuildInterface
}

// InternalBuildInterface has methods to work with InternalBuild resources.
type InternalBuildInterface interface {
	Create(*v1alpha1.InternalBuild) (*v1alpha1.InternalBuild, error)
	Update(*v1alpha1.InternalBuild) (*v1alpha1.InternalBuild, error)
	UpdateStatus(*v1alpha1.InternalBuild) (*v1alpha1.InternalBuild, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.InternalBuild, error)
	List(opts v1.ListOptions) (*v1alpha1.InternalBuildList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.InternalBuild, err error)
	InternalBuildExpansion
}

// internalBuilds implements InternalBuildInterface
type internalBuilds struct {
	client rest.Interface
	ns     string
}

// newInternalBuilds returns a InternalBuilds
func newInternalBuilds(c *BuildV1alpha1Client, namespace string) *internalBuilds {
	return &internalBuilds{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the internalBuild, and returns the corresponding internalBuild object, and an error if there is any.
func (c *internalBuilds) Get(name string, options v1.GetOptions) (result *v1alpha1.InternalBuild, err error) {
	result = &v1alpha1.InternalBuild{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("internalbuilds").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of InternalBuilds that match those selectors.
func (c *internalBuilds) List(opts v1.ListOptions) (result *v1alpha1.InternalBuildList, err error) {
	result = &v1alpha1.InternalBuildList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("internalbuilds").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested internalBuilds.
func (c *internalBuilds) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("internalbuilds").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a internalBuild and creates it.  Returns the server's representation of the internalBuild, and an error, if there is any.
func (c *internalBuilds) Create(internalBuild *v1alpha1.InternalBuild) (result *v1alpha1.InternalBuild, err error) {
	result = &v1alpha1.InternalBuild{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("internalbuilds").
		Body(internalBuild).
		Do().
		Into(result)
	return
}

// Update takes the representation of a internalBuild and updates it. Returns the server's representation of the internalBuild, and an error, if there is any.
func (c *internalBuilds) Update(internalBuild *v1alpha1.InternalBuild) (result *v1alpha1.InternalBuild, err error) {
	result = &v1alpha1.InternalBuild{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("internalbuilds").
		Name(internalBuild.Name).
		Body(internalBuild).
		Do().
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().

func (c *internalBuilds) UpdateStatus(internalBuild *v1alpha1.InternalBuild) (result *v1alpha1.InternalBuild, err error) {
	result = &v1alpha1.InternalBuild{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("internalbuilds").
		Name(internalBuild.Name).
		SubResource("status").
		Body(internalBuild).
		Do().
		Into(result)
	return
}

// Delete takes name of the internalBuild and deletes it. Returns an error if one occurs.
func (c *internalBuilds) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("internalbuilds").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *internalBuilds) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("internalbuilds").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched internalBuild.
func (c *internalBuilds) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.InternalBuild, err error) {
	result = &v1alpha1.InternalBuild{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("internalbuilds").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
