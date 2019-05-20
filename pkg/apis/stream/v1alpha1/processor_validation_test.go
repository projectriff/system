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

func TestValidateProcessor(t *testing.T) {
	for _, c := range []struct {
		name string
		p    *Processor
		want *apis.FieldError
	}{{
		name: "valid",
		p: &Processor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-function",
			},
			Spec: ProcessorSpec{
				FunctionRef: "my-func",
				Inputs:      []string{"my-stream"},
			},
		},
		want: nil,
	}, {
		name: "validates metadata",
		p: &Processor{
			ObjectMeta: metav1.ObjectMeta{},
			Spec: ProcessorSpec{
				FunctionRef: "my-func",
				Inputs:      []string{"my-stream"},
			},
		},
		want: apis.ErrMissingOneOf("metadata.generateName", "metadata.name"),
	}, {
		name: "validates spec",
		p: &Processor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-function",
			},
			Spec: ProcessorSpec{},
		},
		want: apis.ErrMissingField("spec"),
	}} {
		name := c.name
		t.Run(name, func(t *testing.T) {
			got := c.p.Validate(context.Background())
			if diff := cmp.Diff(c.want.Error(), got.Error()); diff != "" {
				t.Errorf("validateProcessor(%s) (-want, +got) = %v", name, diff)
			}
		})
	}
}

func TestValidateProcessorSpec(t *testing.T) {
	for _, c := range []struct {
		name string
		p    *ProcessorSpec
		want *apis.FieldError
	}{{
		name: "valid",
		p: &ProcessorSpec{
			FunctionRef: "my-func",
			Inputs:      []string{"my-stream"},
		},
		want: nil,
	}, {
		name: "empty",
		p:    &ProcessorSpec{},
		want: apis.ErrMissingField(apis.CurrentField),
	}, {
		name: "requires function ref",
		p: &ProcessorSpec{
			FunctionRef: "",
			Inputs:      []string{"my-stream"},
		},
		want: apis.ErrMissingField("function-ref"),
	}, {
		name: "requires inputs",
		p: &ProcessorSpec{
			FunctionRef: "my-func",
			Inputs:      nil,
		},
		want: apis.ErrMissingField("inputs"),
	}, {
		name: "requires valid input",
		p: &ProcessorSpec{
			FunctionRef: "my-func",
			Inputs:      []string{""},
		},
		want: apis.ErrInvalidArrayValue("", "inputs", 0),
	}, {
		name: "requires valid output",
		p: &ProcessorSpec{
			FunctionRef: "my-func",
			Inputs:      []string{"my-stream"},
			Outputs:     []string{""},
		},
		want: apis.ErrInvalidArrayValue("", "outputs", 0),
	}} {
		name := c.name
		t.Run(name, func(t *testing.T) {
			got := c.p.Validate(context.Background())
			if diff := cmp.Diff(c.want.Error(), got.Error()); diff != "" {
				t.Errorf("validateProcessorSpec(%s) (-want, +got) = %v", name, diff)
			}
		})
	}
}
