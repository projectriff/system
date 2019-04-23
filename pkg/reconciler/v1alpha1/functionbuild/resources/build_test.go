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

	"github.com/projectriff/system/pkg/apis/build"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
)

func TestBuild(t *testing.T) {
	fb := createFunctionBuildMeta()
	fb.Labels = map[string]string{testLabelKey: testLabelValue}
	fb.Spec.Image = testImage
	fb.Spec.Source.Git = &buildv1alpha1.GitSource{
		URL:      testGitURL,
		Revision: testGitRevision,
	}
	fb.Status.BuildCacheName = testBuildCacheName
	fb.Spec.Artifact = testFunctionBuildArtifact
	fb.Spec.Handler = testFunctionBuildHandler
	fb.Spec.Invoker = testFunctionBuildInvoker

	b, _ := MakeBuild(fb)

	if errs := b.Validate(context.Background()); errs != nil {
		t.Errorf("expected valid build got errors %+v", errs)
	}

	if got, want := b.Name, testBuildName; got != want {
		t.Errorf("expected %q for build name got %q", want, got)
	}
	if got, want := b.Namespace, testFunctionBuildNamespace; got != want {
		t.Errorf("expected %q for build namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, b.OwnerReferences)

	if got, want := len(b.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := b.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := b.Labels[build.FunctionBuildLabelKey], testFunctionBuildName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if got, want := b.Spec.Template.Name, "riff-cnb"; got != want {
		t.Errorf("expected %q for template name got %q", want, got)
	}
	if got, want := len(b.Spec.Template.Arguments), 5; got != want {
		t.Errorf("expected %q template arg got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[0].Name, "IMAGE"; got != want {
		t.Errorf("expected %q for template image arg name got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[0].Value, testImage; got != want {
		t.Errorf("expected %q for template image arg value got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[1].Name, "FUNCTION_ARTIFACT"; got != want {
		t.Errorf("expected %q for function artifact arg name got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[1].Value, testFunctionBuildArtifact; got != want {
		t.Errorf("expected %q for function artifact arg value got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[2].Name, "FUNCTION_HANDLER"; got != want {
		t.Errorf("expected %q for function handler arg name got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[2].Value, testFunctionBuildHandler; got != want {
		t.Errorf("expected %q for function handler arg value got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[3].Name, "FUNCTION_LANGUAGE"; got != want {
		t.Errorf("expected %q for function language arg name got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[3].Value, testFunctionBuildInvoker; got != want {
		t.Errorf("expected %q for function language arg value got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[4].Name, "CACHE"; got != want {
		t.Errorf("expected %q for template cache arg name got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[4].Value, "cache"; got != want {
		t.Errorf("expected %q for template cache arg value got %q", want, got)
	}

	if got, want := b.Spec.Source.Git.Url, testGitURL; got != want {
		t.Errorf("expected %q for git url got %q", want, got)
	}
	if got, want := b.Spec.Source.Git.Revision, testGitRevision; got != want {
		t.Errorf("expected %q for git revision got %q", want, got)
	}

	if got, want := len(b.Spec.Volumes), 1; got != want {
		t.Errorf("expected %q volume got %q", want, got)
	}
	if got, want := b.Spec.Volumes[0].Name, "cache"; got != want {
		t.Errorf("expected %q for volume name got %q", want, got)
	}
	if got, want := b.Spec.Volumes[0].PersistentVolumeClaim.ClaimName, testBuildCacheName; got != want {
		t.Errorf("expected %q for volume claim name got %q", want, got)
	}
}
