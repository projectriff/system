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
)

func TestProcessorDefault(t *testing.T) {
	tests := []struct {
		name string
		in   *Processor
		want *Processor
	}{{
		name: "empty",
		in:   &Processor{},
		want: &Processor{
			Spec: ProcessorSpec{
				Inputs:  []StreamBinding{},
				Outputs: []StreamBinding{},
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.in
			got.Default()
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("Default (-want, +got) = %v", diff)
			}
		})
	}
}

func TestProcessorSpecDefault(t *testing.T) {
	tests := []struct {
		name string
		in   *ProcessorSpec
		want *ProcessorSpec
	}{{
		name: "empty",
		in:   &ProcessorSpec{},
		want: &ProcessorSpec{
			Inputs:  []StreamBinding{},
			Outputs: []StreamBinding{},
		},
	}, {
		name: "add alias",
		in: &ProcessorSpec{
			Inputs: []StreamBinding{
				{Stream: "my-input"},
			},
			Outputs: []StreamBinding{
				{Stream: "my-output"},
			}},
		want: &ProcessorSpec{
			Inputs: []StreamBinding{
				{Stream: "my-input", Alias: "my-input"},
			},
			Outputs: []StreamBinding{
				{Stream: "my-output", Alias: "my-output"},
			},
		},
	}, {
		name: "preserves alias",
		in: &ProcessorSpec{
			Inputs: []StreamBinding{
				{Stream: "my-input", Alias: "in"},
			},
			Outputs: []StreamBinding{
				{Stream: "my-output", Alias: "out"},
			}},
		want: &ProcessorSpec{
			Inputs: []StreamBinding{
				{Stream: "my-input", Alias: "in"},
			},
			Outputs: []StreamBinding{
				{Stream: "my-output", Alias: "out"},
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.in
			got.Default()
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("Default (-want, +got) = %v", diff)
			}
		})
	}
}
