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
	"github.com/projectriff/system/pkg/apis/build"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestInternalBuild(t *testing.T) {
	fb := createApplicationBuildMeta()
	fb.Labels = map[string]string{testLabelKey: testLabelValue}
	fb.Spec.Image = testImage
	fb.Spec.Source = &buildv1alpha1.Source{
		Git: &buildv1alpha1.GitSource{
			URL:      testGitURL,
			Revision: testGitRevision,
		},
	}
	cacheSize := resource.MustParse(testBuildCacheSize)
	fb.Spec.CacheSize = &cacheSize

	ib, _ := MakeInternalBuild(fb)

	if errs := ib.Validate(context.Background()); errs != nil {
		t.Errorf("expected valid build got errors %+v", errs)
	}

	if got, want := ib.Name, testInternalBuildName; got != want {
		t.Errorf("expected %q for build name got %q", want, got)
	}
	if got, want := ib.Namespace, testApplicationBuildNamespace; got != want {
		t.Errorf("expected %q for build namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, ib.OwnerReferences)

	if got, want := len(ib.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := ib.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := ib.Labels[build.ApplicationBuildLabelKey], testApplicationBuildName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if got, want := ib.Spec.Image, testImage; got != want {
		t.Errorf("expected %q for image got %q", want, got)
	}
	if got, want := ib.Spec.Template, "cf-cnb"; got != want {
		t.Errorf("expected %q for template name got %q", want, got)
	}
	if got, want := len(ib.Spec.Arguments), 0; got != want {
		t.Errorf("expected %q template arg got %q", want, got)
	}
	if diff := cmp.Diff(fb.Spec.Source, ib.Spec.Source); diff != "" {
		t.Errorf("Unexpected source (-want +got): %v", diff)
	}
}
