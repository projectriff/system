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

// FakeFunctionBuilds implements FunctionBuildInterface
type FakeFunctionBuilds struct {
	Fake *FakeBuildV1alpha1
	ns   string
}

var functionbuildsResource = schema.GroupVersionResource{Group: "build.projectriff.io", Version: "v1alpha1", Resource: "functionbuilds"}

var functionbuildsKind = schema.GroupVersionKind{Group: "build.projectriff.io", Version: "v1alpha1", Kind: "FunctionBuild"}

// Get takes name of the functionBuild, and returns the corresponding functionBuild object, and an error if there is any.
func (c *FakeFunctionBuilds) Get(name string, options v1.GetOptions) (result *v1alpha1.FunctionBuild, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(functionbuildsResource, c.ns, name), &v1alpha1.FunctionBuild{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.FunctionBuild), err
}

// List takes label and field selectors, and returns the list of FunctionBuilds that match those selectors.
func (c *FakeFunctionBuilds) List(opts v1.ListOptions) (result *v1alpha1.FunctionBuildList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(functionbuildsResource, functionbuildsKind, c.ns, opts), &v1alpha1.FunctionBuildList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.FunctionBuildList{ListMeta: obj.(*v1alpha1.FunctionBuildList).ListMeta}
	for _, item := range obj.(*v1alpha1.FunctionBuildList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested functionBuilds.
func (c *FakeFunctionBuilds) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(functionbuildsResource, c.ns, opts))

}

// Create takes the representation of a functionBuild and creates it.  Returns the server's representation of the functionBuild, and an error, if there is any.
func (c *FakeFunctionBuilds) Create(functionBuild *v1alpha1.FunctionBuild) (result *v1alpha1.FunctionBuild, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(functionbuildsResource, c.ns, functionBuild), &v1alpha1.FunctionBuild{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.FunctionBuild), err
}

// Update takes the representation of a functionBuild and updates it. Returns the server's representation of the functionBuild, and an error, if there is any.
func (c *FakeFunctionBuilds) Update(functionBuild *v1alpha1.FunctionBuild) (result *v1alpha1.FunctionBuild, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(functionbuildsResource, c.ns, functionBuild), &v1alpha1.FunctionBuild{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.FunctionBuild), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeFunctionBuilds) UpdateStatus(functionBuild *v1alpha1.FunctionBuild) (*v1alpha1.FunctionBuild, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(functionbuildsResource, "status", c.ns, functionBuild), &v1alpha1.FunctionBuild{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.FunctionBuild), err
}

// Delete takes name of the functionBuild and deletes it. Returns an error if one occurs.
func (c *FakeFunctionBuilds) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(functionbuildsResource, c.ns, name), &v1alpha1.FunctionBuild{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeFunctionBuilds) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(functionbuildsResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.FunctionBuildList{})
	return err
}

// Patch applies the patch and returns the patched functionBuild.
func (c *FakeFunctionBuilds) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.FunctionBuild, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(functionbuildsResource, c.ns, name, data, subresources...), &v1alpha1.FunctionBuild{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.FunctionBuild), err
}
