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
	"testing"

	"github.com/google/go-cmp/cmp"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	"github.com/projectriff/system/pkg/apis/run"
	runv1alpha1 "github.com/projectriff/system/pkg/apis/run/v1alpha1"
)

func TestFunctionBuild(t *testing.T) {
	rp := createRequestProcessorMeta()
	rp.Labels = map[string]string{testLabelKey: testLabelValue}
	rp.Spec = append(rp.Spec, runv1alpha1.RequestProcessorSpecItem{
		Build: &runv1alpha1.Build{
			Function: &buildv1alpha1.FunctionBuildSpec{
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

	ab, _ := MakeFunctionBuild(rp, 0)

	if errs := ab.Validate(context.Background()); errs != nil {
		t.Errorf("expected valid function build got errors %+v", errs)
	}

	if got, want := ab.Name, testRequestProcessorName+"-0"; got != want {
		t.Errorf("expected %q for configuration name got %q", want, got)
	}
	if got, want := ab.Namespace, testRequestProcessorNamespace; got != want {
		t.Errorf("expected %q for configuration namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, ab.OwnerReferences)

	if got, want := len(ab.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := ab.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := ab.Labels[run.RequestProcessorLabelKey], testRequestProcessorName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if diff := cmp.Diff(*rp.Spec[0].Build.Function, ab.Spec); diff != "" {
		t.Errorf("podspec volumes differ (-want, +got) = %v", diff)
	}
}
