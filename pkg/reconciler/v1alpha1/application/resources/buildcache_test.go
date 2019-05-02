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

	"github.com/projectriff/system/pkg/apis/build"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestBuildCache(t *testing.T) {
	a := createApplicationMeta()
	a.Labels = map[string]string{testLabelKey: testLabelValue}
	cacheSize := resource.MustParse(testBuildCacheSize)
	a.Spec.CacheSize = &cacheSize

	c, _ := MakeBuildCache(a)

	if got, want := c.Name, testBuildCacheName; got != want {
		t.Errorf("expected %q for build cache name got %q", want, got)
	}
	if got, want := c.Namespace, testApplicationNamespace; got != want {
		t.Errorf("expected %q for build cache namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, c.OwnerReferences)

	if got, want := len(c.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := c.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := c.Labels[build.ApplicationLabelKey], testApplicationName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}

	cacheSize = c.Spec.Resources.Requests[corev1.ResourceStorage]
	if got, want := cacheSize.String(), testBuildCacheSize; got != want {
		t.Errorf("expected %q build cache size got %q", want, got)
	}
}
