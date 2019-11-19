/*
Copyright 2019 the original author or authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"

	v1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
)

// FakePulsarProviders implements PulsarProviderInterface
type FakePulsarProviders struct {
	Fake *FakeStreamingV1alpha1
	ns   string
}

var pulsarprovidersResource = schema.GroupVersionResource{Group: "streaming.projectriff.io", Version: "v1alpha1", Resource: "pulsarproviders"}

var pulsarprovidersKind = schema.GroupVersionKind{Group: "streaming.projectriff.io", Version: "v1alpha1", Kind: "PulsarProvider"}

// Get takes name of the pulsarProvider, and returns the corresponding pulsarProvider object, and an error if there is any.
func (c *FakePulsarProviders) Get(name string, options v1.GetOptions) (result *v1alpha1.PulsarProvider, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(pulsarprovidersResource, c.ns, name), &v1alpha1.PulsarProvider{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PulsarProvider), err
}

// List takes label and field selectors, and returns the list of PulsarProviders that match those selectors.
func (c *FakePulsarProviders) List(opts v1.ListOptions) (result *v1alpha1.PulsarProviderList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(pulsarprovidersResource, pulsarprovidersKind, c.ns, opts), &v1alpha1.PulsarProviderList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.PulsarProviderList{ListMeta: obj.(*v1alpha1.PulsarProviderList).ListMeta}
	for _, item := range obj.(*v1alpha1.PulsarProviderList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested pulsarProviders.
func (c *FakePulsarProviders) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(pulsarprovidersResource, c.ns, opts))

}

// Create takes the representation of a pulsarProvider and creates it.  Returns the server's representation of the pulsarProvider, and an error, if there is any.
func (c *FakePulsarProviders) Create(pulsarProvider *v1alpha1.PulsarProvider) (result *v1alpha1.PulsarProvider, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(pulsarprovidersResource, c.ns, pulsarProvider), &v1alpha1.PulsarProvider{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PulsarProvider), err
}

// Update takes the representation of a pulsarProvider and updates it. Returns the server's representation of the pulsarProvider, and an error, if there is any.
func (c *FakePulsarProviders) Update(pulsarProvider *v1alpha1.PulsarProvider) (result *v1alpha1.PulsarProvider, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(pulsarprovidersResource, c.ns, pulsarProvider), &v1alpha1.PulsarProvider{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PulsarProvider), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakePulsarProviders) UpdateStatus(pulsarProvider *v1alpha1.PulsarProvider) (*v1alpha1.PulsarProvider, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(pulsarprovidersResource, "status", c.ns, pulsarProvider), &v1alpha1.PulsarProvider{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PulsarProvider), err
}

// Delete takes name of the pulsarProvider and deletes it. Returns an error if one occurs.
func (c *FakePulsarProviders) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(pulsarprovidersResource, c.ns, name), &v1alpha1.PulsarProvider{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakePulsarProviders) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(pulsarprovidersResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.PulsarProviderList{})
	return err
}

// Patch applies the patch and returns the patched pulsarProvider.
func (c *FakePulsarProviders) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.PulsarProvider, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(pulsarprovidersResource, c.ns, name, pt, data, subresources...), &v1alpha1.PulsarProvider{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PulsarProvider), err
}