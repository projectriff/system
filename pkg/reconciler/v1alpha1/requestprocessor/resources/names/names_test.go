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

package names

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	requestv1alpha1 "github.com/projectriff/system/pkg/apis/request/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNamer(t *testing.T) {
	tests := []struct {
		name string
		request  *requestv1alpha1.RequestProcessor
		f    func(*requestv1alpha1.RequestProcessor) string
		want string
	}{{
		name: "Route",
		request: &requestv1alpha1.RequestProcessor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
		},
		f:    Route,
		want: "foo",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.f(test.request)
			if got != test.want {
				t.Errorf("%s() = %v, wanted %v", test.name, got, test.want)
			}
		})
	}
}

func TestNameItems(t *testing.T) {
	tests := []struct {
		name string
		request  *requestv1alpha1.RequestProcessor
		f    func(*requestv1alpha1.RequestProcessor) []string
		want []string
	}{{
		name: "Items empty",
		request: &requestv1alpha1.RequestProcessor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
		},
		f:    Items,
		want: []string{},
	}, {
		name: "Items single",
		request: &requestv1alpha1.RequestProcessor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: requestv1alpha1.RequestProcessorSpec{
				{Name: "bar"},
			},
		},
		f:    Items,
		want: []string{"foo-bar"},
	}, {
		name: "Items many",
		request: &requestv1alpha1.RequestProcessor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: requestv1alpha1.RequestProcessorSpec{
				{Name: "bar"},
				{Name: "baz"},
			},
		},
		f: Items,
		want: []string{
			"foo-bar",
			"foo-baz",
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.f(test.request)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("%s (-want, +got) = %v", test.name, diff)
			}
		})
	}
}
