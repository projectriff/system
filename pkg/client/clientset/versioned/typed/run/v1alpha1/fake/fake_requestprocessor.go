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
package fake

import (
	v1alpha1 "github.com/projectriff/system/pkg/apis/run/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeRequestProcessors implements RequestProcessorInterface
type FakeRequestProcessors struct {
	Fake *FakeRunV1alpha1
	ns   string
}

var requestprocessorsResource = schema.GroupVersionResource{Group: "run.projectriff.io", Version: "v1alpha1", Resource: "requestprocessors"}

var requestprocessorsKind = schema.GroupVersionKind{Group: "run.projectriff.io", Version: "v1alpha1", Kind: "RequestProcessor"}

// Get takes name of the requestProcessor, and returns the corresponding requestProcessor object, and an error if there is any.
func (c *FakeRequestProcessors) Get(name string, options v1.GetOptions) (result *v1alpha1.RequestProcessor, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(requestprocessorsResource, c.ns, name), &v1alpha1.RequestProcessor{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.RequestProcessor), err
}

// List takes label and field selectors, and returns the list of RequestProcessors that match those selectors.
func (c *FakeRequestProcessors) List(opts v1.ListOptions) (result *v1alpha1.RequestProcessorList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(requestprocessorsResource, requestprocessorsKind, c.ns, opts), &v1alpha1.RequestProcessorList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.RequestProcessorList{ListMeta: obj.(*v1alpha1.RequestProcessorList).ListMeta}
	for _, item := range obj.(*v1alpha1.RequestProcessorList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested requestProcessors.
func (c *FakeRequestProcessors) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(requestprocessorsResource, c.ns, opts))

}

// Create takes the representation of a requestProcessor and creates it.  Returns the server's representation of the requestProcessor, and an error, if there is any.
func (c *FakeRequestProcessors) Create(requestProcessor *v1alpha1.RequestProcessor) (result *v1alpha1.RequestProcessor, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(requestprocessorsResource, c.ns, requestProcessor), &v1alpha1.RequestProcessor{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.RequestProcessor), err
}

// Update takes the representation of a requestProcessor and updates it. Returns the server's representation of the requestProcessor, and an error, if there is any.
func (c *FakeRequestProcessors) Update(requestProcessor *v1alpha1.RequestProcessor) (result *v1alpha1.RequestProcessor, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(requestprocessorsResource, c.ns, requestProcessor), &v1alpha1.RequestProcessor{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.RequestProcessor), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeRequestProcessors) UpdateStatus(requestProcessor *v1alpha1.RequestProcessor) (*v1alpha1.RequestProcessor, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(requestprocessorsResource, "status", c.ns, requestProcessor), &v1alpha1.RequestProcessor{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.RequestProcessor), err
}

// Delete takes name of the requestProcessor and deletes it. Returns an error if one occurs.
func (c *FakeRequestProcessors) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(requestprocessorsResource, c.ns, name), &v1alpha1.RequestProcessor{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeRequestProcessors) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(requestprocessorsResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.RequestProcessorList{})
	return err
}

// Patch applies the patch and returns the patched requestProcessor.
func (c *FakeRequestProcessors) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.RequestProcessor, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(requestprocessorsResource, c.ns, name, data, subresources...), &v1alpha1.RequestProcessor{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.RequestProcessor), err
}
