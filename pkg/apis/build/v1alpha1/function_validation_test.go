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

package v1alpha1

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/apis"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateFunction(t *testing.T) {
	for _, c := range []struct {
		name  string
		build *Function
		want  *apis.FieldError
	}{{
		name: "valid",
		build: &Function{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-function",
			},
			Spec: FunctionSpec{
				Image: "test-image",
				Source: Source{
					Git: &GitSource{
						URL:      "https://example.com/repo.git",
						Revision: "master",
					},
				},
			},
		},
		want: nil,
	}, {
		name: "validates metadata",
		build: &Function{
			ObjectMeta: metav1.ObjectMeta{},
			Spec: FunctionSpec{
				Image: "test-image",
				Source: Source{
					Git: &GitSource{
						URL:      "https://example.com/repo.git",
						Revision: "master",
					},
				},
			},
		},
		want: apis.ErrMissingOneOf("metadata.generateName", "metadata.name"),
	}, {
		name: "validates spec",
		build: &Function{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-function",
			},
			Spec: FunctionSpec{},
		},
		want: apis.ErrMissingField("spec"),
	}} {
		name := c.name
		t.Run(name, func(t *testing.T) {
			got := c.build.Validate(context.Background())
			if diff := cmp.Diff(c.want.Error(), got.Error()); diff != "" {
				t.Errorf("validateFunction(%s) (-want, +got) = %v", name, diff)
			}
		})
	}
}

func TestValidateFunctionSpec(t *testing.T) {
	for _, c := range []struct {
		name  string
		build *FunctionSpec
		want  *apis.FieldError
	}{{
		name: "valid",
		build: &FunctionSpec{
			Image: "test-image",
			Source: Source{
				Git: &GitSource{
					URL:      "https://example.com/repo.git",
					Revision: "master",
				},
			},
		},
		want: nil,
	}, {
		name:  "empty",
		build: &FunctionSpec{},
		want:  apis.ErrMissingField(apis.CurrentField),
	}, {
		name: "requires image",
		build: &FunctionSpec{
			Source: Source{
				Git: &GitSource{
					URL:      "https://example.com/repo.git",
					Revision: "master",
				},
			},
		},
		want: apis.ErrMissingField("image"),
	}, {
		name: "requires source",
		build: &FunctionSpec{
			Image: "test-image",
		},
		want: apis.ErrMissingField("source"),
	}} {
		name := c.name
		t.Run(name, func(t *testing.T) {
			got := c.build.Validate(context.Background())
			if diff := cmp.Diff(c.want.Error(), got.Error()); diff != "" {
				t.Errorf("validateFunctionSpec(%s) (-want, +got) = %v", name, diff)
			}
		})
	}
}
