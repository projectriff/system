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
				Inputs: []StreamBinding{
					{Stream: "my-stream", Alias: "in"},
				},
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
			Inputs: []StreamBinding{
				{Stream: "my-stream", Alias: "in"},
			},
		},
		expected: validation.FieldErrors{},
	}, {
		name: "requires function ref",
		target: &ProcessorSpec{
			FunctionRef: "",
			Inputs: []StreamBinding{
				{Stream: "my-stream", Alias: "in"},
			},
		},
		expected: validation.ErrMissingField("functionRef"),
	}, {
		name: "requires inputs",
		target: &ProcessorSpec{
			FunctionRef: "my-func",
		},
		expected: validation.ErrMissingField("inputs"),
	}, {
		name: "empty input",
		target: &ProcessorSpec{
			FunctionRef: "my-func",
			Inputs: []StreamBinding{
				{},
			},
		},
		expected: validation.FieldErrors{}.Also(
			validation.ErrMissingField("inputs[0].stream"),
			validation.ErrMissingField("inputs[0].alias"),
		),
	}, {
		name: "valid input",
		target: &ProcessorSpec{
			FunctionRef: "my-func",
			Inputs: []StreamBinding{
				{Stream: "my-stream", Alias: "in"},
			},
		},
		expected: validation.FieldErrors{},
	}, {
		name: "empty output",
		target: &ProcessorSpec{
			FunctionRef: "my-func",
			Inputs: []StreamBinding{
				{Stream: "my-stream", Alias: "in"},
			},
			Outputs: []StreamBinding{
				{},
			},
		},
		expected: validation.FieldErrors{}.Also(
			validation.ErrMissingField("outputs[0].stream"),
			validation.ErrMissingField("outputs[0].alias"),
		),
	}, {
		name: "valid output",
		target: &ProcessorSpec{
			FunctionRef: "my-func",
			Inputs: []StreamBinding{
				{Stream: "my-stream", Alias: "my-alias"},
			},
			Outputs: []StreamBinding{
				{Stream: "my-stream", Alias: "my-alias"},
			},
		},
		expected: validation.FieldErrors{},
	}} {
		t.Run(c.name, func(t *testing.T) {
			actual := c.target.Validate()
			if diff := cmp.Diff(c.expected, actual); diff != "" {
				t.Errorf("validateProcessorSpec(%s) (-expected, +actual) = %v", c.name, diff)
			}
		})
	}
}
