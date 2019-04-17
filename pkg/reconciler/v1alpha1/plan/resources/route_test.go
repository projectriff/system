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

	"github.com/projectriff/system/pkg/apis/plan"
	projectriffv1alpha1 "github.com/projectriff/system/pkg/apis/projectriff/v1alpha1"
)

func TestRoute(t *testing.T) {
	p := createPlanMeta()
	p.Labels = map[string]string{testLabelKey: testLabelValue}
	p.Spec.Run = &projectriffv1alpha1.Run{}
	p.Status.LatestImage = testImage
	p.Status.ConfigurationName = testConfigurationName

	r, _ := MakeRoute(p)

	if errs := r.Validate(context.Background()); errs != nil {
		t.Errorf("expected valid route got errors %+v", errs)
	}

	if got, want := r.Name, testPlanName; got != want {
		t.Errorf("expected %q for route name got %q", want, got)
	}
	if got, want := r.Namespace, testPlanNamespace; got != want {
		t.Errorf("expected %q for route namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, r.OwnerReferences)

	if got, want := len(r.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := r.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := r.Labels[plan.PlanLabelKey], testPlanName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if got, want := len(r.Spec.Traffic), 1; got != want {
		t.Errorf("expected %q route traffic entry got %q", want, got)
	}
	if got, want := r.Spec.Traffic[0].ConfigurationName, testConfigurationName; got != want {
		t.Errorf("expected %q for route traffic target got %q", want, got)
	}
}
