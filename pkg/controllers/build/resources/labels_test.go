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

package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
)

func TestMakeFunctionLabels(t *testing.T) {
	tests := []struct {
		name  string
		build *buildv1alpha1.Function
		want  map[string]string
	}{{
		name: "just application name",
		build: &buildv1alpha1.Function{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
			},
		},
		want: map[string]string{
			buildv1alpha1.FunctionLabelKey: "bar",
		},
	}, {
		name: "pass through labels",
		build: &buildv1alpha1.Function{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "baz",
				Name:      "blah",
				Labels: map[string]string{
					"asdf": "bazinga",
					"ooga": "booga",
				},
			},
		},
		want: map[string]string{
			buildv1alpha1.FunctionLabelKey: "blah",
			"asdf":                         "bazinga",
			"ooga":                         "booga",
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := makeFunctionLabels(test.build)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("makeLabels (-want, +got) = %v", diff)
			}
		})
	}
}

func TestMakeApplicationLabels(t *testing.T) {
	tests := []struct {
		name  string
		build *buildv1alpha1.Application
		want  map[string]string
	}{{
		name: "just application name",
		build: &buildv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
			},
		},
		want: map[string]string{
			buildv1alpha1.ApplicationLabelKey: "bar",
		},
	}, {
		name: "pass through labels",
		build: &buildv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "baz",
				Name:      "blah",
				Labels: map[string]string{
					"asdf": "bazinga",
					"ooga": "booga",
				},
			},
		},
		want: map[string]string{
			buildv1alpha1.ApplicationLabelKey: "blah",
			"asdf":                            "bazinga",
			"ooga":                            "booga",
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := makeApplicationLabels(test.build)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("makeLabels (-want, +got) = %v", diff)
			}
		})
	}
}
