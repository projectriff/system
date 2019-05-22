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
	"testing"

	"github.com/projectriff/system/pkg/apis/request"
)

func TestRoute(t *testing.T) {
	r := createRouteMeta()
	r.Labels = map[string]string{testLabelKey: testLabelValue}
	r.Status.KnConfigurationName = testKnConfigurationName

	knr, _ := MakeKnRoute(r)

	if got, want := knr.Name, testKnRouteName; got != want {
		t.Errorf("expected %q for route name got %q", want, got)
	}
	if got, want := knr.Namespace, testRouteNamespace; got != want {
		t.Errorf("expected %q for route namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, knr.OwnerReferences)

	if got, want := len(knr.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := knr.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := knr.Labels[request.RouteLabelKey], testRouteName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if got, want := len(knr.Spec.Traffic), 1; got != want {
		t.Errorf("expected %d traffic policy got %d", want, got)
	}
	if got, want := knr.Spec.Traffic[0].Name, ""; got != want {
		t.Errorf("expected %q traffic policy tag got %q", want, got)
	}
	if got, want := knr.Spec.Traffic[0].Percent, 100; got != want {
		t.Errorf("expected %q traffic policy tag got %q", want, got)
	}
	if got, want := knr.Spec.Traffic[0].ConfigurationName, testKnConfigurationName; got != want {
		t.Errorf("expected %q traffic policy configuration got %q", want, got)
	}
}
