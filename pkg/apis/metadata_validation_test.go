/*
Copyright 2018 The Knative Authors

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

package apis

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/apis"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateObjectMetadata(t *testing.T) {
	for _, c := range []struct {
		name string
		meta *metav1.ObjectMeta
		want *apis.FieldError
	}{{
		name: "valid name",
		meta: &metav1.ObjectMeta{
			Name: "test-resource",
		},
		want: nil,
	}, {
		name: "valid generatename",
		meta: &metav1.ObjectMeta{
			GenerateName: "test-resource",
		},
		want: nil,
	}, {
		name: "name or generatename required",
		meta: &metav1.ObjectMeta{},
		want: apis.ErrMissingOneOf("generateName", "name"),
	}, {
		name: "name and generatename allowed",
		meta: &metav1.ObjectMeta{
			Name:         "test-resource-8a3bd",
			GenerateName: "test-resource-",
		},
		want: nil,
	}, {
		name: "name is too long",
		meta: &metav1.ObjectMeta{
			Name: strings.Repeat("s", maxLength+1),
		},
		want: apis.ErrInvalidValue("length must be no more than 63 characters", "name"),
	}, {
		name: "generatename is too long",
		meta: &metav1.ObjectMeta{
			GenerateName: strings.Repeat("s", maxLength+1),
		},
		want: apis.ErrInvalidValue("length must be no more than 63 characters", "generateName"),
	}, {
		name: "name invalid characters",
		meta: &metav1.ObjectMeta{
			Name: "bad.name",
		},
		want: apis.ErrInvalidValue("special character . must not be present", "name"),
	}, {
		name: "generatename invalid characters",
		meta: &metav1.ObjectMeta{
			GenerateName: "bad.name",
		},
		want: apis.ErrInvalidValue("special character . must not be present", "generateName"),
	}} {
		name := c.name
		t.Run(name, func(t *testing.T) {
			got := ValidateObjectMetadata(c.meta)
			if diff := cmp.Diff(c.want.Error(), got.Error()); diff != "" {
				t.Errorf("validateObjectMetadata(%s) (-want, +got) = %v", name, diff)
			}
		})
	}
}
