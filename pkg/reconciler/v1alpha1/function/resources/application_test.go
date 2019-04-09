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
	"github.com/projectriff/system/pkg/apis/projectriff"
	projectriffv1alpha1 "github.com/projectriff/system/pkg/apis/projectriff/v1alpha1"
)

func TestFunction(t *testing.T) {
	f := createFunctionMeta()
	f.Labels = map[string]string{testLabelKey: testLabelValue}
	f.Spec.Image = testImage
	f.Spec.Build.Artifact = testFunctionBuildArtifact
	f.Spec.Build.Handler = testFunctionBuildHandler
	f.Spec.Build.Invoker = testFunctionBuildInvoker
	f.Spec.Build.Source = &projectriffv1alpha1.Source{}

	a, _ := MakeApplication(f)

	if errs := a.Validate(context.Background()); errs != nil {
		t.Errorf("expected valid application got errors %+v", errs)
	}

	if got, want := a.Name, testApplicationName; got != want {
		t.Errorf("expected %q for application name got %q", want, got)
	}
	if got, want := a.Namespace, testFunctionNamespace; got != want {
		t.Errorf("expected %q for application namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, a.OwnerReferences)

	if got, want := len(a.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := a.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := a.Labels[projectriff.FunctionLabelKey], testFunctionName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if got, want := a.Spec.Image, testImage; got != want {
		t.Errorf("expected %q for application image got %q", want, got)
	}
	if got, want := a.Spec.Build.Template, "riff-cnb"; got != want {
		t.Errorf("expected %q for build template got %q", want, got)
	}
	if diff := cmp.Diff(f.Spec.Build.Source, a.Spec.Build.Source); diff != "" {
		t.Errorf("Unexpected build source (-want +got): %v", diff)
	}
	if diff := cmp.Diff([]projectriffv1alpha1.BuildArgument{
		{Name: "FUNCTION_ARTIFACT", Value: testFunctionBuildArtifact},
		{Name: "FUNCTION_HANDLER", Value: testFunctionBuildHandler},
		{Name: "FUNCTION_LANGUAGE", Value: testFunctionBuildInvoker},
	}, a.Spec.Build.Arguments); diff != "" {
		t.Errorf("Unexpected build arguments (-want +got): %v", diff)
	}
	if diff := cmp.Diff(f.Spec.Run.EnvFrom, a.Spec.Run.EnvFrom); diff != "" {
		t.Errorf("Unexpected run envfrom (-want +got): %v", diff)
	}
	if diff := cmp.Diff(f.Spec.Run.Env, a.Spec.Run.Env); diff != "" {
		t.Errorf("Unexpected run env (-want +got): %v", diff)
	}
	if diff := cmp.Diff(f.Spec.Run.Resources, a.Spec.Run.Resources); diff != "" {
		t.Errorf("Unexpected run resources (-want +got): %v", diff)
	}
}

// TODO reenable
// func TestMalformed(t *testing.T) {
// 	f := createFunctionMeta()
// 	a, err := MakeApplication(f)
// 	if err == nil {
// 		t.Errorf("MakeApplication() = %v, wanted error", a)
// 	}
// }
