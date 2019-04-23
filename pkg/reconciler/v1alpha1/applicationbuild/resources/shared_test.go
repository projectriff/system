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
	"github.com/google/go-cmp/cmp/cmpopts"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testApplicationBuildName      = "test-applicationbuild"
	testApplicationBuildNamespace = "test-applicationbuild-namespace"
	testBuildName                 = "test-applicationbuild-application"
	testLabelKey                  = "test-label-key"
	testLabelValue                = "test-label-value"
	testImage                     = "test-image"
	testGitURL                    = "https://example.com/repo.git"
	testGitRevision               = "master"
	testBuildCacheSize            = "8Gi"
	testBuildCacheName            = "build-cache-test-applicationbuild-application"
)

func expectOwnerReferencesSetCorrectly(t *testing.T, ownerRefs []metav1.OwnerReference) {
	if got, want := len(ownerRefs), 1; got != want {
		t.Errorf("expected %d owner refs got %d", want, got)
		return
	}

	expectedRefs := []metav1.OwnerReference{{
		APIVersion: "build.projectriff.io/v1alpha1",
		Kind:       "ApplicationBuild",
		Name:       testApplicationBuildName,
	}}
	if diff := cmp.Diff(expectedRefs, ownerRefs, cmpopts.IgnoreFields(expectedRefs[0], "Controller", "BlockOwnerDeletion")); diff != "" {
		t.Errorf("Unexpected applicationbuild owner refs diff (-want +got): %v", diff)
	}
}

func createApplicationBuildMeta() *buildv1alpha1.ApplicationBuild {
	return &buildv1alpha1.ApplicationBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testApplicationBuildName,
			Namespace: testApplicationBuildNamespace,
		},
	}
}
