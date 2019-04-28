/*
Copyright 2019 The Knative Authors

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
)

func TestRequestProcessorDefaulting(t *testing.T) {
	tests := []struct {
		name string
		in   *RequestProcessor
		want *RequestProcessor
	}{{
		name: "empty",
		in:   &RequestProcessor{},
		want: &RequestProcessor{},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.in
			got.SetDefaults(context.Background())
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("SetDefaults (-want, +got) = %v", diff)
			}
		})
	}
}

func TestRequestProcessorSpecDefaulting(t *testing.T) {
	tests := []struct {
		name string
		in   *RequestProcessorSpec
		want *RequestProcessorSpec
	}{{
		name: "empty",
		in:   &RequestProcessorSpec{},
		want: &RequestProcessorSpec{},
	}, {
		name: "ensure at least one container",
		in: &RequestProcessorSpec{
			{
				Percent: makePint(100),
			},
		},
		want: &RequestProcessorSpec{
			{
				Percent: makePint(100),
				PodSpec: makeDefaultPodSpec(),
			},
		},
	}, {
		name: "default traffic",
		in: &RequestProcessorSpec{
			{
				PodSpec: makeDefaultPodSpec(),
			},
		},
		want: &RequestProcessorSpec{
			{
				Percent: makePint(100),
				PodSpec: makeDefaultPodSpec(),
			},
		},
	}, {
		name: "default partially unallocated traffic",
		in: &RequestProcessorSpec{
			{
				Percent: makePint(90),
				PodSpec: makeDefaultPodSpec(),
			},
			{
				PodSpec: makeDefaultPodSpec(),
			},
		},
		want: &RequestProcessorSpec{
			{
				Percent: makePint(90),
				PodSpec: makeDefaultPodSpec(),
			},
			{
				Percent: makePint(10),
				PodSpec: makeDefaultPodSpec(),
			},
		},
	}, {
		name: "default partially unallocated traffic evenly",
		in: &RequestProcessorSpec{
			{
				Percent: makePint(90),
				PodSpec: makeDefaultPodSpec(),
			},
			{
				PodSpec: makeDefaultPodSpec(),
			},
			{
				PodSpec: makeDefaultPodSpec(),
			},
		},
		want: &RequestProcessorSpec{
			{
				Percent: makePint(90),
				PodSpec: makeDefaultPodSpec(),
			},
			{
				Percent: makePint(5),
				PodSpec: makeDefaultPodSpec(),
			},
			{
				Percent: makePint(5),
				PodSpec: makeDefaultPodSpec(),
			},
		},
	}, {
		name: "default partially unallocated traffic as evenly as possible",
		in: &RequestProcessorSpec{
			{
				Percent: makePint(95),
				PodSpec: makeDefaultPodSpec(),
			},
			{
				PodSpec: makeDefaultPodSpec(),
			},
			{
				PodSpec: makeDefaultPodSpec(),
			},
		},
		want: &RequestProcessorSpec{
			{
				Percent: makePint(95),
				PodSpec: makeDefaultPodSpec(),
			},
			{
				Percent: makePint(3),
				PodSpec: makeDefaultPodSpec(),
			},
			{
				Percent: makePint(2),
				PodSpec: makeDefaultPodSpec(),
			},
		},
	}, {
		name: "an item can intentionally receive no traffic",
		in: &RequestProcessorSpec{
			{
				Percent: makePint(0),
				PodSpec: makeDefaultPodSpec(),
			},
			{
				Percent: makePint(95),
				PodSpec: makeDefaultPodSpec(),
			},
			{
				PodSpec: makeDefaultPodSpec(),
			},
		},
		want: &RequestProcessorSpec{
			{
				Percent: makePint(0),
				PodSpec: makeDefaultPodSpec(),
			},
			{
				Percent: makePint(95),
				PodSpec: makeDefaultPodSpec(),
			},
			{
				Percent: makePint(5),
				PodSpec: makeDefaultPodSpec(),
			},
		},
	}, {
		name: "an item can unintentionally receive no traffic",
		in: &RequestProcessorSpec{
			{
				Percent: makePint(99),
				PodSpec: makeDefaultPodSpec(),
			},
			{
				PodSpec: makeDefaultPodSpec(),
			},
			{
				PodSpec: makeDefaultPodSpec(),
			},
		},
		want: &RequestProcessorSpec{
			{
				Percent: makePint(99),
				PodSpec: makeDefaultPodSpec(),
			},
			{
				Percent: makePint(1),
				PodSpec: makeDefaultPodSpec(),
			},
			{
				Percent: makePint(0),
				PodSpec: makeDefaultPodSpec(),
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.in
			got.SetDefaults(context.Background())
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("SetDefaults (-want, +got) = %v", diff)
			}
		})
	}
}
