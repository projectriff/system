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
	"github.com/projectriff/system/pkg/apis/core"
)

func TestDeployment(t *testing.T) {
	d := createDeployerMeta()
	d.Labels = map[string]string{testLabelKey: testLabelValue}
	d.Spec.Template.ServiceAccountName = testServiceAccount
	d.Spec.Template.Containers[0].Image = testImage

	kd, _ := MakeDeployment(d)

	if got, want := kd.Name, testDeploymentName; got != want {
		t.Errorf("expected %q for deployment name got %q", want, got)
	}
	if got, want := kd.Namespace, testDeployerNamespace; got != want {
		t.Errorf("expected %q for deployment namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, kd.OwnerReferences)

	if got, want := len(kd.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := kd.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := kd.Labels[core.DeployerLabelKey], testDeployerName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if diff := cmp.Diff(kd.Labels, kd.Spec.Template.Labels); diff != "" {
		t.Errorf("deployment and pod template labels differ (-want, +got) = %v", diff)
	}

	if diff := cmp.Diff(d.Spec.Template, &kd.Spec.Template.Spec); diff != "" {
		t.Errorf("deployment and deployer pod templates differ (-want, +got) = %v", diff)
	}
}
