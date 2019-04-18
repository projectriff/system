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
	commonv1alpha1 "github.com/projectriff/system/pkg/apis/common/v1alpha1"
	"github.com/projectriff/system/pkg/apis/projectriff"
	projectriffv1alpha1 "github.com/projectriff/system/pkg/apis/projectriff/v1alpha1"
)

func TestFunction(t *testing.T) {
	f := createFunctionMeta()
	f.Labels = map[string]string{testLabelKey: testLabelValue}
	f.Spec.Image = testImage
	f.Spec.Build = &projectriffv1alpha1.FunctionBuild{
		Artifact: testFunctionBuildArtifact,
		Handler:  testFunctionBuildHandler,
		Invoker:  testFunctionBuildInvoker,
		Source:   &commonv1alpha1.Source{},
	}
	f.Spec.Run = &commonv1alpha1.Run{}

	p, _ := MakePlan(f)

	if errs := p.Validate(context.Background()); errs != nil {
		t.Errorf("expected valid application got errors %+v", errs)
	}

	if got, want := p.Name, testApplicationName; got != want {
		t.Errorf("expected %q for application name got %q", want, got)
	}
	if got, want := p.Namespace, testFunctionNamespace; got != want {
		t.Errorf("expected %q for application namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, p.OwnerReferences)

	if got, want := len(p.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := p.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := p.Labels[projectriff.FunctionLabelKey], testFunctionName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if got, want := p.Spec.Image, testImage; got != want {
		t.Errorf("expected %q for application image got %q", want, got)
	}
	if got, want := p.Spec.Build.Template, "riff-cnb"; got != want {
		t.Errorf("expected %q for build template got %q", want, got)
	}
	if diff := cmp.Diff(f.Spec.Build.Source, p.Spec.Build.Source); diff != "" {
		t.Errorf("Unexpected build source (-want +got): %v", diff)
	}
	if diff := cmp.Diff([]commonv1alpha1.BuildArgument{
		{Name: "FUNCTION_ARTIFACT", Value: testFunctionBuildArtifact},
		{Name: "FUNCTION_HANDLER", Value: testFunctionBuildHandler},
		{Name: "FUNCTION_LANGUAGE", Value: testFunctionBuildInvoker},
	}, p.Spec.Build.Arguments); diff != "" {
		t.Errorf("Unexpected build arguments (-want +got): %v", diff)
	}
	if diff := cmp.Diff(f.Spec.Run.EnvFrom, p.Spec.Run.EnvFrom); diff != "" {
		t.Errorf("Unexpected run envfrom (-want +got): %v", diff)
	}
	if diff := cmp.Diff(f.Spec.Run.Env, p.Spec.Run.Env); diff != "" {
		t.Errorf("Unexpected run env (-want +got): %v", diff)
	}
	if diff := cmp.Diff(f.Spec.Run.Resources, p.Spec.Run.Resources); diff != "" {
		t.Errorf("Unexpected run resources (-want +got): %v", diff)
	}
}
