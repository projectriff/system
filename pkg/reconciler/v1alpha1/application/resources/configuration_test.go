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

	"github.com/projectriff/system/pkg/apis/projectriff"
)

func TestConfiguration(t *testing.T) {
	a := createApplicationMeta()
	a.Labels = map[string]string{testLabelKey: testLabelValue}
	a.Spec.Image = testImage

	c, _ := MakeConfiguration(a)

	if errs := c.Validate(context.Background()); errs != nil {
		t.Errorf("expected valid configuration got errors %+v", errs)
	}

	if got, want := c.Name, testApplicationName; got != want {
		t.Errorf("expected %q for configuration name got %q", want, got)
	}
	if got, want := c.Namespace, testApplicationNamespace; got != want {
		t.Errorf("expected %q for configuration namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, c.OwnerReferences)

	if got, want := len(c.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := c.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := c.Labels[projectriff.ApplicationLabelKey], testApplicationName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	if got, want := c.Spec.RevisionTemplate.Spec.Container.Image, testImage; got != want {
		t.Errorf("expected %q for configuration image got %q", want, got)
	}
}

// TODO reenable
// func TestConfiguration_Malformed(t *testing.T) {
// 	a := createApplicationMeta()
// 	c, err := MakeConfiguration(a)
// 	if err == nil {
// 		t.Errorf("MakeConfiguration() = %v, wanted error", s)
// 	}
// }
