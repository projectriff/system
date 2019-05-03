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
	v1alpha1 "github.com/projectriff/system/pkg/apis/stream/v1alpha1"
	scheme "github.com/projectriff/system/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// StreamProcessorsGetter has a method to return a StreamProcessorInterface.
// A group's client should implement this interface.
type StreamProcessorsGetter interface {
	StreamProcessors(namespace string) StreamProcessorInterface
}

// StreamProcessorInterface has methods to work with StreamProcessor resources.
type StreamProcessorInterface interface {
	Create(*v1alpha1.StreamProcessor) (*v1alpha1.StreamProcessor, error)
	Update(*v1alpha1.StreamProcessor) (*v1alpha1.StreamProcessor, error)
	UpdateStatus(*v1alpha1.StreamProcessor) (*v1alpha1.StreamProcessor, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.StreamProcessor, error)
	List(opts v1.ListOptions) (*v1alpha1.StreamProcessorList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.StreamProcessor, err error)
	StreamProcessorExpansion
}

// streamProcessors implements StreamProcessorInterface
type streamProcessors struct {
	client rest.Interface
	ns     string
}

// newStreamProcessors returns a StreamProcessors
func newStreamProcessors(c *StreamV1alpha1Client, namespace string) *streamProcessors {
	return &streamProcessors{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the streamProcessor, and returns the corresponding streamProcessor object, and an error if there is any.
func (c *streamProcessors) Get(name string, options v1.GetOptions) (result *v1alpha1.StreamProcessor, err error) {
	result = &v1alpha1.StreamProcessor{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("streamprocessors").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of StreamProcessors that match those selectors.
func (c *streamProcessors) List(opts v1.ListOptions) (result *v1alpha1.StreamProcessorList, err error) {
	result = &v1alpha1.StreamProcessorList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("streamprocessors").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested streamProcessors.
func (c *streamProcessors) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("streamprocessors").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a streamProcessor and creates it.  Returns the server's representation of the streamProcessor, and an error, if there is any.
func (c *streamProcessors) Create(streamProcessor *v1alpha1.StreamProcessor) (result *v1alpha1.StreamProcessor, err error) {
	result = &v1alpha1.StreamProcessor{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("streamprocessors").
		Body(streamProcessor).
		Do().
		Into(result)
	return
}

// Update takes the representation of a streamProcessor and updates it. Returns the server's representation of the streamProcessor, and an error, if there is any.
func (c *streamProcessors) Update(streamProcessor *v1alpha1.StreamProcessor) (result *v1alpha1.StreamProcessor, err error) {
	result = &v1alpha1.StreamProcessor{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("streamprocessors").
		Name(streamProcessor.Name).
		Body(streamProcessor).
		Do().
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().

func (c *streamProcessors) UpdateStatus(streamProcessor *v1alpha1.StreamProcessor) (result *v1alpha1.StreamProcessor, err error) {
	result = &v1alpha1.StreamProcessor{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("streamprocessors").
		Name(streamProcessor.Name).
		SubResource("status").
		Body(streamProcessor).
		Do().
		Into(result)
	return
}

// Delete takes name of the streamProcessor and deletes it. Returns an error if one occurs.
func (c *streamProcessors) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("streamprocessors").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *streamProcessors) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("streamprocessors").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched streamProcessor.
func (c *streamProcessors) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.StreamProcessor, err error) {
	result = &v1alpha1.StreamProcessor{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("streamprocessors").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
