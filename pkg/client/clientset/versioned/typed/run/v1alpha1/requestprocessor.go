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
	v1alpha1 "github.com/projectriff/system/pkg/apis/run/v1alpha1"
	scheme "github.com/projectriff/system/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// RequestProcessorsGetter has a method to return a RequestProcessorInterface.
// A group's client should implement this interface.
type RequestProcessorsGetter interface {
	RequestProcessors(namespace string) RequestProcessorInterface
}

// RequestProcessorInterface has methods to work with RequestProcessor resources.
type RequestProcessorInterface interface {
	Create(*v1alpha1.RequestProcessor) (*v1alpha1.RequestProcessor, error)
	Update(*v1alpha1.RequestProcessor) (*v1alpha1.RequestProcessor, error)
	UpdateStatus(*v1alpha1.RequestProcessor) (*v1alpha1.RequestProcessor, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.RequestProcessor, error)
	List(opts v1.ListOptions) (*v1alpha1.RequestProcessorList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.RequestProcessor, err error)
	RequestProcessorExpansion
}

// requestProcessors implements RequestProcessorInterface
type requestProcessors struct {
	client rest.Interface
	ns     string
}

// newRequestProcessors returns a RequestProcessors
func newRequestProcessors(c *RunV1alpha1Client, namespace string) *requestProcessors {
	return &requestProcessors{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the requestProcessor, and returns the corresponding requestProcessor object, and an error if there is any.
func (c *requestProcessors) Get(name string, options v1.GetOptions) (result *v1alpha1.RequestProcessor, err error) {
	result = &v1alpha1.RequestProcessor{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("requestprocessors").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of RequestProcessors that match those selectors.
func (c *requestProcessors) List(opts v1.ListOptions) (result *v1alpha1.RequestProcessorList, err error) {
	result = &v1alpha1.RequestProcessorList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("requestprocessors").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested requestProcessors.
func (c *requestProcessors) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("requestprocessors").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a requestProcessor and creates it.  Returns the server's representation of the requestProcessor, and an error, if there is any.
func (c *requestProcessors) Create(requestProcessor *v1alpha1.RequestProcessor) (result *v1alpha1.RequestProcessor, err error) {
	result = &v1alpha1.RequestProcessor{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("requestprocessors").
		Body(requestProcessor).
		Do().
		Into(result)
	return
}

// Update takes the representation of a requestProcessor and updates it. Returns the server's representation of the requestProcessor, and an error, if there is any.
func (c *requestProcessors) Update(requestProcessor *v1alpha1.RequestProcessor) (result *v1alpha1.RequestProcessor, err error) {
	result = &v1alpha1.RequestProcessor{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("requestprocessors").
		Name(requestProcessor.Name).
		Body(requestProcessor).
		Do().
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().

func (c *requestProcessors) UpdateStatus(requestProcessor *v1alpha1.RequestProcessor) (result *v1alpha1.RequestProcessor, err error) {
	result = &v1alpha1.RequestProcessor{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("requestprocessors").
		Name(requestProcessor.Name).
		SubResource("status").
		Body(requestProcessor).
		Do().
		Into(result)
	return
}

// Delete takes name of the requestProcessor and deletes it. Returns an error if one occurs.
func (c *requestProcessors) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("requestprocessors").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *requestProcessors) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("requestprocessors").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched requestProcessor.
func (c *requestProcessors) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.RequestProcessor, err error) {
	result = &v1alpha1.RequestProcessor{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("requestprocessors").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
