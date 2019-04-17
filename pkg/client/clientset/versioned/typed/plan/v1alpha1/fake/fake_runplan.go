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

// FakeRunPlans implements RunPlanInterface
type FakeRunPlans struct {
	Fake *FakePlanV1alpha1
	ns   string
}

var runplansResource = schema.GroupVersionResource{Group: "plan.internal.projectriff.io", Version: "v1alpha1", Resource: "runplans"}

var runplansKind = schema.GroupVersionKind{Group: "plan.internal.projectriff.io", Version: "v1alpha1", Kind: "RunPlan"}

// Get takes name of the runPlan, and returns the corresponding runPlan object, and an error if there is any.
func (c *FakeRunPlans) Get(name string, options v1.GetOptions) (result *v1alpha1.RunPlan, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(runplansResource, c.ns, name), &v1alpha1.RunPlan{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.RunPlan), err
}

// List takes label and field selectors, and returns the list of RunPlans that match those selectors.
func (c *FakeRunPlans) List(opts v1.ListOptions) (result *v1alpha1.RunPlanList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(runplansResource, runplansKind, c.ns, opts), &v1alpha1.RunPlanList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.RunPlanList{ListMeta: obj.(*v1alpha1.RunPlanList).ListMeta}
	for _, item := range obj.(*v1alpha1.RunPlanList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested runPlans.
func (c *FakeRunPlans) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(runplansResource, c.ns, opts))

}

// Create takes the representation of a runPlan and creates it.  Returns the server's representation of the runPlan, and an error, if there is any.
func (c *FakeRunPlans) Create(runPlan *v1alpha1.RunPlan) (result *v1alpha1.RunPlan, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(runplansResource, c.ns, runPlan), &v1alpha1.RunPlan{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.RunPlan), err
}

// Update takes the representation of a runPlan and updates it. Returns the server's representation of the runPlan, and an error, if there is any.
func (c *FakeRunPlans) Update(runPlan *v1alpha1.RunPlan) (result *v1alpha1.RunPlan, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(runplansResource, c.ns, runPlan), &v1alpha1.RunPlan{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.RunPlan), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeRunPlans) UpdateStatus(runPlan *v1alpha1.RunPlan) (*v1alpha1.RunPlan, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(runplansResource, "status", c.ns, runPlan), &v1alpha1.RunPlan{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.RunPlan), err
}

// Delete takes name of the runPlan and deletes it. Returns an error if one occurs.
func (c *FakeRunPlans) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(runplansResource, c.ns, name), &v1alpha1.RunPlan{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeRunPlans) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(runplansResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.RunPlanList{})
	return err
}

// Patch applies the patch and returns the patched runPlan.
func (c *FakeRunPlans) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.RunPlan, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(runplansResource, c.ns, name, data, subresources...), &v1alpha1.RunPlan{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.RunPlan), err
}
