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
	v1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeApplicationBuilds implements ApplicationBuildInterface
type FakeApplicationBuilds struct {
	Fake *FakeBuildV1alpha1
	ns   string
}

var applicationbuildsResource = schema.GroupVersionResource{Group: "build.projectriff.io", Version: "v1alpha1", Resource: "applicationbuilds"}

var applicationbuildsKind = schema.GroupVersionKind{Group: "build.projectriff.io", Version: "v1alpha1", Kind: "ApplicationBuild"}

// Get takes name of the applicationBuild, and returns the corresponding applicationBuild object, and an error if there is any.
func (c *FakeApplicationBuilds) Get(name string, options v1.GetOptions) (result *v1alpha1.ApplicationBuild, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(applicationbuildsResource, c.ns, name), &v1alpha1.ApplicationBuild{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ApplicationBuild), err
}

// List takes label and field selectors, and returns the list of ApplicationBuilds that match those selectors.
func (c *FakeApplicationBuilds) List(opts v1.ListOptions) (result *v1alpha1.ApplicationBuildList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(applicationbuildsResource, applicationbuildsKind, c.ns, opts), &v1alpha1.ApplicationBuildList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.ApplicationBuildList{ListMeta: obj.(*v1alpha1.ApplicationBuildList).ListMeta}
	for _, item := range obj.(*v1alpha1.ApplicationBuildList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested applicationBuilds.
func (c *FakeApplicationBuilds) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(applicationbuildsResource, c.ns, opts))

}

// Create takes the representation of a applicationBuild and creates it.  Returns the server's representation of the applicationBuild, and an error, if there is any.
func (c *FakeApplicationBuilds) Create(applicationBuild *v1alpha1.ApplicationBuild) (result *v1alpha1.ApplicationBuild, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(applicationbuildsResource, c.ns, applicationBuild), &v1alpha1.ApplicationBuild{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ApplicationBuild), err
}

// Update takes the representation of a applicationBuild and updates it. Returns the server's representation of the applicationBuild, and an error, if there is any.
func (c *FakeApplicationBuilds) Update(applicationBuild *v1alpha1.ApplicationBuild) (result *v1alpha1.ApplicationBuild, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(applicationbuildsResource, c.ns, applicationBuild), &v1alpha1.ApplicationBuild{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ApplicationBuild), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeApplicationBuilds) UpdateStatus(applicationBuild *v1alpha1.ApplicationBuild) (*v1alpha1.ApplicationBuild, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(applicationbuildsResource, "status", c.ns, applicationBuild), &v1alpha1.ApplicationBuild{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ApplicationBuild), err
}

// Delete takes name of the applicationBuild and deletes it. Returns an error if one occurs.
func (c *FakeApplicationBuilds) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(applicationbuildsResource, c.ns, name), &v1alpha1.ApplicationBuild{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeApplicationBuilds) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(applicationbuildsResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.ApplicationBuildList{})
	return err
}

// Patch applies the patch and returns the patched applicationBuild.
func (c *FakeApplicationBuilds) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.ApplicationBuild, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(applicationbuildsResource, c.ns, name, data, subresources...), &v1alpha1.ApplicationBuild{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ApplicationBuild), err
}
