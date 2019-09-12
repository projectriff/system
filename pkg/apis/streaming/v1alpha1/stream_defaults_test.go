/*
Copyright 2019 the original author or authors.

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

package v1alpha1

import "testing"

func TestStreamDefaults(t *testing.T) {
	expectedContentType := "application/octet-stream"
	stream := Stream{}

	stream.SetDefaults(nil)

	actualContentType := stream.Spec.ContentType
	if actualContentType != expectedContentType {
		t.Errorf("expected default stream content-type to be %s, got %s", expectedContentType, actualContentType)
	}
}

func TestStreamDefaultsDoNotOverride(t *testing.T) {
	expectedContentType := "application/x-doom"
	stream := Stream{
		Spec: StreamSpec{
			ContentType: expectedContentType,
		},
	}

	stream.SetDefaults(nil)

	actualContentType := stream.Spec.ContentType
	if actualContentType != expectedContentType {
		t.Errorf("expected stream content-type to be %s, got %s", expectedContentType, actualContentType)
	}
}
