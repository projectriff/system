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
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	"github.com/projectriff/system/pkg/apis/request"
	requestv1alpha1 "github.com/projectriff/system/pkg/apis/request/v1alpha1"
)

func TestFunction(t *testing.T) {
	rp := createRequestProcessorMeta()
	rp.Labels = map[string]string{testLabelKey: testLabelValue}
	rp.Spec = append(rp.Spec, requestv1alpha1.RequestProcessorSpecItem{
		Name: testItemName,
		Build: &requestv1alpha1.Build{
			Function: &buildv1alpha1.FunctionSpec{
				Image: "example.com/repo",
				Source: buildv1alpha1.Source{
					Git: &buildv1alpha1.GitSource{
						URL:      "https://example.com/repo.git",
						Revision: "master",
					},
				},
			},
		},
	})

	f, _ := MakeFunction(rp, 0)

	if errs := f.Validate(context.Background()); errs != nil {
		t.Errorf("expected valid function got errors %+v", errs)
	}

	if got, want := f.Name, fmt.Sprintf("%s-%s", testRequestProcessorName, testItemName); got != want {
		t.Errorf("expected %q for configuration name got %q", want, got)
	}
	if got, want := f.Namespace, testRequestProcessorNamespace; got != want {
		t.Errorf("expected %q for configuration namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, f.OwnerReferences)

	if got, want := len(f.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := f.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := f.Labels[request.RequestProcessorLabelKey], testRequestProcessorName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if diff := cmp.Diff(*rp.Spec[0].Build.Function, f.Spec); diff != "" {
		t.Errorf("podspec volumes differ (-want, +got) = %v", diff)
	}
}
