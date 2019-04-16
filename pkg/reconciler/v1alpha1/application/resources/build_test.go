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

	"github.com/projectriff/system/pkg/apis/projectriff"
	projectriffv1alpha1 "github.com/projectriff/system/pkg/apis/projectriff/v1alpha1"
)

func TestBuild(t *testing.T) {
	a := createApplicationMeta()
	a.Labels = map[string]string{testLabelKey: testLabelValue}
	a.Spec.Image = testImage
	a.Spec.Build.Template = testBuildTemplate
	a.Spec.Build.Arguments = []projectriffv1alpha1.BuildArgument{
		{Name: testBuildArgName, Value: testBuildArgValue},
	}
	a.Spec.Build.Source = &projectriffv1alpha1.Source{
		Git: &projectriffv1alpha1.GitSource{
			URL:      testGitURL,
			Revision: testGitRevision,
		},
	}
	a.Status.BuildCacheName = testBuildCacheName

	b, _ := MakeBuild(a)

	if errs := b.Validate(context.Background()); errs != nil {
		t.Errorf("expected valid build got errors %+v", errs)
	}

	if got, want := b.Name, testBuildName; got != want {
		t.Errorf("expected %q for build name got %q", want, got)
	}
	if got, want := b.Namespace, testApplicationNamespace; got != want {
		t.Errorf("expected %q for build namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, b.OwnerReferences)

	if got, want := len(b.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := b.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := b.Labels[projectriff.ApplicationLabelKey], testApplicationName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if got, want := b.Spec.Template.Name, testBuildTemplate; got != want {
		t.Errorf("expected %q for template name got %q", want, got)
	}
	if got, want := len(b.Spec.Template.Arguments), 3; got != want {
		t.Errorf("expected %q template arg got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[0].Name, "IMAGE"; got != want {
		t.Errorf("expected %q for template image arg name got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[0].Value, testImage; got != want {
		t.Errorf("expected %q for template image arg value got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[1].Name, "CACHE"; got != want {
		t.Errorf("expected %q for template cache arg name got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[1].Value, "cache"; got != want {
		t.Errorf("expected %q for template cache arg value got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[2].Name, testBuildArgName; got != want {
		t.Errorf("expected %q for template custom arg name got %q", want, got)
	}
	if got, want := b.Spec.Template.Arguments[2].Value, testBuildArgValue; got != want {
		t.Errorf("expected %q for template custom arg value got %q", want, got)
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
