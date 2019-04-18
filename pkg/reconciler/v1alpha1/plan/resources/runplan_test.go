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
	commonv1alpha1 "github.com/projectriff/system/pkg/apis/common/v1alpha1"
	"github.com/projectriff/system/pkg/apis/plan"
)

func TestRunPlan(t *testing.T) {
	p := createPlanMeta()
	p.Labels = map[string]string{testLabelKey: testLabelValue}
	p.Spec.Run = &commonv1alpha1.Run{}
	p.Status.LatestImage = testImage

	rp, _ := MakeRunPlan(p)

	if errs := rp.Validate(context.Background()); errs != nil {
		t.Errorf("expected valid runplan got errors %+v", errs)
	}

	if got, want := rp.Name, testPlanName; got != want {
		t.Errorf("expected %q for runplan name got %q", want, got)
	}
	if got, want := rp.Namespace, testPlanNamespace; got != want {
		t.Errorf("expected %q for runplan namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, rp.OwnerReferences)

	if got, want := len(rp.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := rp.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := rp.Labels[plan.PlanLabelKey], testPlanName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if got, want := rp.Spec.Image, testImage; got != want {
		t.Errorf("expected %q for image got %q", want, got)
	}
	if diff := cmp.Diff(p.Spec.Run, &rp.Spec.Run); diff != "" {
		t.Errorf("run (-want, +got) = %v", diff)
	}
}
