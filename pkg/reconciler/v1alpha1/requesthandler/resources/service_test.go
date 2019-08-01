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

func TestService(t *testing.T) {
	h := createHandlerMeta()
	h.Labels = map[string]string{testLabelKey: testLabelValue}
	h.Status.DeploymentName = testDeploymentName

	s, _ := MakeService(h)

	if got, want := s.Name, testServiceName; got != want {
		t.Errorf("expected %q for service name got %q", want, got)
	}
	if got, want := s.Namespace, testHandlerNamespace; got != want {
		t.Errorf("expected %q for service namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, s.OwnerReferences)

	if got, want := len(s.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := s.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := s.Labels[request.HandlerLabelKey], testHandlerName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if got, want := len(s.Spec.Selector), 1; got != want {
		t.Errorf("expected %d selector labels got %d", want, got)
	}
	if got, want := s.Spec.Selector[request.HandlerLabelKey], testHandlerName; got != want {
		t.Errorf("expected %q selector labels got %q", want, got)
	}
}
