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
)

func TestService(t *testing.T) {
	a := createApplicationMeta()
	a.Labels = map[string]string{testLabelKey: testLabelValue}
	a.Spec.Image = testImage

	s, _ := MakeService(a)

	if errs := s.Validate(context.Background()); errs != nil {
		t.Errorf("expected valid service got errors %+v", errs)
	}

	if got, want := a.Name, testApplicationName; got != want {
		t.Errorf("expected %q for service name got %q", want, got)
	}
	if got, want := a.Namespace, testApplicationNamespace; got != want {
		t.Errorf("expected %q for service namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, s.OwnerReferences)

	if got, want := len(s.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := s.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := s.Labels[projectriff.ApplicationLabelKey], testApplicationName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if got, want := s.Spec.RunLatest.Configuration.RevisionTemplate.Spec.Container.Image, testImage; got != want {
		t.Errorf("expected %q for service image got %q", want, got)
	}
}

// TODO reenable
// func TestService_Malformed(t *testing.T) {
// 	a := createApplicationMeta()
// 	s, err := MakeService(a)
// 	if err == nil {
// 		t.Errorf("MakeService() = %v, wanted error", s)
// 	}
// }
