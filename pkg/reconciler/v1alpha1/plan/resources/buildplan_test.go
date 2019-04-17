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
	"github.com/projectriff/system/pkg/apis/plan"
	projectriffv1alpha1 "github.com/projectriff/system/pkg/apis/projectriff/v1alpha1"
)

func TestBuildPlan(t *testing.T) {
	p := createPlanMeta()
	p.Labels = map[string]string{testLabelKey: testLabelValue}
	p.Spec.Image = testImage
	p.Spec.Build = &projectriffv1alpha1.Build{}

	bp, _ := MakeBuildPlan(p)

	if errs := bp.Validate(context.Background()); errs != nil {
		t.Errorf("expected valid buildplan got errors %+v", errs)
	}

	if got, want := bp.Name, testPlanName; got != want {
		t.Errorf("expected %q for buildplan name got %q", want, got)
	}
	if got, want := bp.Namespace, testPlanNamespace; got != want {
		t.Errorf("expected %q for buildplan namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, bp.OwnerReferences)

	if got, want := len(bp.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := bp.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := bp.Labels[plan.PlanLabelKey], testPlanName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if got, want := bp.Spec.Image, p.Spec.Image; got != want {
		t.Errorf("expected %q for image got %q", want, got)
	}
	if diff := cmp.Diff(p.Spec.Build, &bp.Spec.Build); diff != "" {
		t.Errorf("build (-want, +got) = %v", diff)
	}
}
