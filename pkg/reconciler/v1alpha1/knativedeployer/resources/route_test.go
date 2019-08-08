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

	"github.com/projectriff/system/pkg/apis/knative"
)

func TestRoute(t *testing.T) {
	c := createDeployerMeta()
	c.Labels = map[string]string{testLabelKey: testLabelValue}
	c.Status.ConfigurationName = testConfigurationName

	r, _ := MakeRoute(c)

	if got, want := r.Name, testRouteName; got != want {
		t.Errorf("expected %q for route name got %q", want, got)
	}
	if got, want := r.Namespace, testDeployerNamespace; got != want {
		t.Errorf("expected %q for route namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, r.OwnerReferences)

	if got, want := len(r.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := r.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := r.Labels[knative.DeployerLabelKey], testDeployerName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if got, want := len(r.Spec.Traffic), 1; got != want {
		t.Errorf("expected %d traffic policy got %d", want, got)
	}
	if got, want := r.Spec.Traffic[0].Tag, ""; got != want {
		t.Errorf("expected %q traffic policy tag got %q", want, got)
	}
	if got, want := r.Spec.Traffic[0].Percent, 100; got != want {
		t.Errorf("expected %q traffic policy tag got %q", want, got)
	}
	if got, want := r.Spec.Traffic[0].ConfigurationName, testConfigurationName; got != want {
		t.Errorf("expected %q traffic policy configuration got %q", want, got)
	}
}
