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
	"github.com/projectriff/system/pkg/apis/run"
	runv1alpha1 "github.com/projectriff/system/pkg/apis/run/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func TestBuild(t *testing.T) {
	rp := createRequestProcessorMeta()
	rp.Labels = map[string]string{testLabelKey: testLabelValue}
	rp.Spec = append(rp.Spec, runv1alpha1.RequestProcessorSpecItem{
		PodSpec: corev1.PodSpec{
			ServiceAccountName: testServiceAccount,
			Containers: []corev1.Container{
				{
					Image: testImage,
				},
			},
		},
	})

	c, _ := MakeConfiguration(rp, 0)

	if errs := c.Validate(context.Background()); errs != nil {
		t.Errorf("expected valid configuration got errors %+v", errs)
	}

	if got, want := c.Name, testConfigurationName; got != want {
		t.Errorf("expected %q for configuration name got %q", want, got)
	}
	if got, want := c.Namespace, testRequestProcessorNamespace; got != want {
		t.Errorf("expected %q for configuration namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, c.OwnerReferences)

	if got, want := len(c.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := c.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := c.Labels[run.RequestProcessorLabelKey], testRequestProcessorName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if got, want := c.Spec.RevisionTemplate.Spec.ServiceAccountName, testServiceAccount; got != want {
		t.Errorf("expected %q for service account got %q", want, got)
	}
	if got, want := c.Spec.RevisionTemplate.Spec.Container.Image, testImage; got != want {
		t.Errorf("expected %q for image got %q", want, got)
	}
	if diff := cmp.Diff(rp.Spec[0].Volumes, c.Spec.RevisionTemplate.Spec.Volumes); diff != "" {
		t.Errorf("podspec volumes differ (-want, +got) = %v", diff)
	}
}
