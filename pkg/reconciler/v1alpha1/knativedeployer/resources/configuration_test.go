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
	"github.com/projectriff/system/pkg/apis/knative"
)

func TestBuild(t *testing.T) {
	c := createDeployerMeta()
	c.Labels = map[string]string{testLabelKey: testLabelValue}
	c.Spec.Template.ServiceAccountName = testServiceAccount
	c.Spec.Template.Containers[0].Image = testImage

	kc, _ := MakeConfiguration(c)

	if errs := kc.Validate(context.Background()); errs != nil {
		t.Errorf("expected valid configuration got errors %+v", errs)
	}

	if got, want := kc.Name, testConfigurationName; got != want {
		t.Errorf("expected %q for configuration name got %q", want, got)
	}
	if got, want := kc.Namespace, testDeployerNamespace; got != want {
		t.Errorf("expected %q for configuration namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, kc.OwnerReferences)

	if got, want := len(kc.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := kc.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := kc.Labels[knative.DeployerLabelKey], testDeployerName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if diff := cmp.Diff(kc.Labels, kc.Spec.Template.Labels); diff != "" {
		t.Errorf("configuration and revision template labels differ (-want, +got) = %v", diff)
	}

	if got, want := kc.Spec.Template.Spec.ServiceAccountName, testServiceAccount; got != want {
		t.Errorf("expected %q for service account got %q", want, got)
	}
	if got, want := kc.Spec.Template.Spec.Containers[0].Image, testImage; got != want {
		t.Errorf("expected %q for image got %q", want, got)
	}
	if diff := cmp.Diff(c.Spec.Template.Volumes, kc.Spec.Template.Spec.Volumes); diff != "" {
		t.Errorf("podspec volumes differ (-want, +got) = %v", diff)
	}
}
