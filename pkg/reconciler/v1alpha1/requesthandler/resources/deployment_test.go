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

	"github.com/google/go-cmp/cmp"
	"github.com/projectriff/system/pkg/apis/request"
)

func TestDeployment(t *testing.T) {
	h := createHandlerMeta()
	h.Labels = map[string]string{testLabelKey: testLabelValue}
	h.Spec.Template.ServiceAccountName = testServiceAccount
	h.Spec.Template.Containers[0].Image = testImage

	d, _ := MakeDeployment(h)

	if got, want := d.Name, testDeploymentName; got != want {
		t.Errorf("expected %q for deployment name got %q", want, got)
	}
	if got, want := d.Namespace, testHandlerNamespace; got != want {
		t.Errorf("expected %q for deployment namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, d.OwnerReferences)

	if got, want := len(d.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := d.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := d.Labels[request.HandlerLabelKey], testHandlerName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if diff := cmp.Diff(d.Labels, d.Spec.Template.Labels); diff != "" {
		t.Errorf("deployment and pod template labels differ (-want, +got) = %v", diff)
	}

	if diff := cmp.Diff(h.Spec.Template, &d.Spec.Template.Spec); diff != "" {
		t.Errorf("deployment and handler pod templates differ (-want, +got) = %v", diff)
	}
}
