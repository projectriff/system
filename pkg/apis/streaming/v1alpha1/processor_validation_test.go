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

package v1alpha1

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/projectriff/system/pkg/validation"
)

func TestValidateProcessor(t *testing.T) {
	for _, c := range []struct {
		name     string
		target   *Processor
		expected validation.FieldErrors
	}{{
		name:     "empty",
		target:   &Processor{},
		expected: validation.ErrMissingField("spec"),
	}, {
		name: "valid",
		target: &Processor{
			Spec: ProcessorSpec{
				FunctionRef: "my-func",
				Inputs:      []string{"my-stream"},
			},
		},
		expected: validation.FieldErrors{},
	}} {
		t.Run(c.name, func(t *testing.T) {
			actual := c.target.Validate()
			if diff := cmp.Diff(c.expected, actual); diff != "" {
				t.Errorf("validateProcessor(%s) (-expected, +actual) = %v", c.name, diff)
			}
		})
	}
}

func TestValidateProcessorSpec(t *testing.T) {
	for _, c := range []struct {
		name     string
		target   *ProcessorSpec
		expected validation.FieldErrors
	}{{
		name:     "empty",
		target:   &ProcessorSpec{},
		expected: validation.ErrMissingField(validation.CurrentField),
	}, {
		name: "valid",
		target: &ProcessorSpec{
			FunctionRef: "my-func",
			Inputs:      []string{"my-stream"},
		},
		expected: validation.FieldErrors{},
	}, {
		name: "requires function ref",
		target: &ProcessorSpec{
			FunctionRef: "",
			Inputs:      []string{"my-stream"},
		},
		expected: validation.ErrMissingField("functionRef"),
	}, {
		name: "requires inputs",
		target: &ProcessorSpec{
			FunctionRef: "my-func",
			Inputs:      nil,
		},
		expected: validation.ErrMissingField("inputs"),
	}, {
		name: "requires valid input",
		target: &ProcessorSpec{
			FunctionRef: "my-func",
			Inputs:      []string{""},
		},
		expected: validation.ErrInvalidArrayValue("", "inputs", 0),
	}, {
		name: "validates output",
		target: &ProcessorSpec{
			FunctionRef: "my-func",
			Inputs:      []string{"my-stream"},
			Outputs:     []string{""},
		},
		expected: validation.ErrInvalidArrayValue("", "outputs", 0),
	}} {
		t.Run(c.name, func(t *testing.T) {
			actual := c.target.Validate()
			if diff := cmp.Diff(c.expected, actual); diff != "" {
				t.Errorf("validateProcessorSpec(%s) (-expected, +actual) = %v", c.name, diff)
			}
		})
	}
}
