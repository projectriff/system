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
	v1alpha1 "github.com/projectriff/system/pkg/apis/knative/v1alpha1"
	scheme "github.com/projectriff/system/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// ConfigurersGetter has a method to return a ConfigurerInterface.
// A group's client should implement this interface.
type ConfigurersGetter interface {
	Configurers(namespace string) ConfigurerInterface
}

// ConfigurerInterface has methods to work with Configurer resources.
type ConfigurerInterface interface {
	Create(*v1alpha1.Configurer) (*v1alpha1.Configurer, error)
	Update(*v1alpha1.Configurer) (*v1alpha1.Configurer, error)
	UpdateStatus(*v1alpha1.Configurer) (*v1alpha1.Configurer, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.Configurer, error)
	List(opts v1.ListOptions) (*v1alpha1.ConfigurerList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Configurer, err error)
	ConfigurerExpansion
}

// configurers implements ConfigurerInterface
type configurers struct {
	client rest.Interface
	ns     string
}

// newConfigurers returns a Configurers
func newConfigurers(c *KnativeV1alpha1Client, namespace string) *configurers {
	return &configurers{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the configurer, and returns the corresponding configurer object, and an error if there is any.
func (c *configurers) Get(name string, options v1.GetOptions) (result *v1alpha1.Configurer, err error) {
	result = &v1alpha1.Configurer{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("configurers").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of Configurers that match those selectors.
func (c *configurers) List(opts v1.ListOptions) (result *v1alpha1.ConfigurerList, err error) {
	result = &v1alpha1.ConfigurerList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("configurers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested configurers.
func (c *configurers) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("configurers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a configurer and creates it.  Returns the server's representation of the configurer, and an error, if there is any.
func (c *configurers) Create(configurer *v1alpha1.Configurer) (result *v1alpha1.Configurer, err error) {
	result = &v1alpha1.Configurer{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("configurers").
		Body(configurer).
		Do().
		Into(result)
	return
}

// Update takes the representation of a configurer and updates it. Returns the server's representation of the configurer, and an error, if there is any.
func (c *configurers) Update(configurer *v1alpha1.Configurer) (result *v1alpha1.Configurer, err error) {
	result = &v1alpha1.Configurer{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("configurers").
		Name(configurer.Name).
		Body(configurer).
		Do().
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().

func (c *configurers) UpdateStatus(configurer *v1alpha1.Configurer) (result *v1alpha1.Configurer, err error) {
	result = &v1alpha1.Configurer{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("configurers").
		Name(configurer.Name).
		SubResource("status").
		Body(configurer).
		Do().
		Into(result)
	return
}

// Delete takes name of the configurer and deletes it. Returns an error if one occurs.
func (c *configurers) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("configurers").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *configurers) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("configurers").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched configurer.
func (c *configurers) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Configurer, err error) {
	result = &v1alpha1.Configurer{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("configurers").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
