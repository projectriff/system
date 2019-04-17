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
	v1alpha1 "github.com/projectriff/system/pkg/apis/plan/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeBuildPlans implements BuildPlanInterface
type FakeBuildPlans struct {
	Fake *FakePlanV1alpha1
	ns   string
}

var buildplansResource = schema.GroupVersionResource{Group: "plan.internal.projectriff.io", Version: "v1alpha1", Resource: "buildplans"}

var buildplansKind = schema.GroupVersionKind{Group: "plan.internal.projectriff.io", Version: "v1alpha1", Kind: "BuildPlan"}

// Get takes name of the buildPlan, and returns the corresponding buildPlan object, and an error if there is any.
func (c *FakeBuildPlans) Get(name string, options v1.GetOptions) (result *v1alpha1.BuildPlan, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(buildplansResource, c.ns, name), &v1alpha1.BuildPlan{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.BuildPlan), err
}

// List takes label and field selectors, and returns the list of BuildPlans that match those selectors.
func (c *FakeBuildPlans) List(opts v1.ListOptions) (result *v1alpha1.BuildPlanList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(buildplansResource, buildplansKind, c.ns, opts), &v1alpha1.BuildPlanList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.BuildPlanList{ListMeta: obj.(*v1alpha1.BuildPlanList).ListMeta}
	for _, item := range obj.(*v1alpha1.BuildPlanList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested buildPlans.
func (c *FakeBuildPlans) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(buildplansResource, c.ns, opts))

}

// Create takes the representation of a buildPlan and creates it.  Returns the server's representation of the buildPlan, and an error, if there is any.
func (c *FakeBuildPlans) Create(buildPlan *v1alpha1.BuildPlan) (result *v1alpha1.BuildPlan, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(buildplansResource, c.ns, buildPlan), &v1alpha1.BuildPlan{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.BuildPlan), err
}

// Update takes the representation of a buildPlan and updates it. Returns the server's representation of the buildPlan, and an error, if there is any.
func (c *FakeBuildPlans) Update(buildPlan *v1alpha1.BuildPlan) (result *v1alpha1.BuildPlan, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(buildplansResource, c.ns, buildPlan), &v1alpha1.BuildPlan{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.BuildPlan), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeBuildPlans) UpdateStatus(buildPlan *v1alpha1.BuildPlan) (*v1alpha1.BuildPlan, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(buildplansResource, "status", c.ns, buildPlan), &v1alpha1.BuildPlan{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.BuildPlan), err
}

// Delete takes name of the buildPlan and deletes it. Returns an error if one occurs.
func (c *FakeBuildPlans) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(buildplansResource, c.ns, name), &v1alpha1.BuildPlan{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeBuildPlans) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(buildplansResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.BuildPlanList{})
	return err
}

// Patch applies the patch and returns the patched buildPlan.
func (c *FakeBuildPlans) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.BuildPlan, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(buildplansResource, c.ns, name, data, subresources...), &v1alpha1.BuildPlan{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.BuildPlan), err
}
