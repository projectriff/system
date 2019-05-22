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
	"github.com/projectriff/system/pkg/apis/request"
)

func TestBuild(t *testing.T) {
	r := createRouteMeta()
	r.Labels = map[string]string{testLabelKey: testLabelValue}
	r.Spec.Template.ServiceAccountName = testServiceAccount
	r.Spec.Template.Containers[0].Image = testImage

	knc, _ := MakeKnConfiguration(r)

	if errs := knc.Validate(context.Background()); errs != nil {
		t.Errorf("expected valid configuration got errors %+v", errs)
	}

	if got, want := knc.Name, testKnConfigurationName; got != want {
		t.Errorf("expected %q for configuration name got %q", want, got)
	}
	if got, want := knc.Namespace, testRouteNamespace; got != want {
		t.Errorf("expected %q for configuration namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, knc.OwnerReferences)

	if got, want := len(knc.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := knc.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := knc.Labels[request.RouteLabelKey], testRouteName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if got, want := knc.Spec.RevisionTemplate.Spec.ServiceAccountName, testServiceAccount; got != want {
		t.Errorf("expected %q for service account got %q", want, got)
	}
	if got, want := knc.Spec.RevisionTemplate.Spec.Container.Image, testImage; got != want {
		t.Errorf("expected %q for image got %q", want, got)
	}
	if diff := cmp.Diff(r.Spec.Template.Volumes, knc.Spec.RevisionTemplate.Spec.Volumes); diff != "" {
		t.Errorf("podspec volumes differ (-want, +got) = %v", diff)
	}
}
