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

package v1alpha1

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/apis"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateStream(t *testing.T) {
	for _, c := range []struct {
		name string
		s    *Stream
		want *apis.FieldError
	}{{
		name: "valid",
		s: &Stream{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-function",
			},
			Spec: StreamSpec{
				Provider:    "kafka",
				ContentType: "application/json",
			},
		},
		want: nil,
	}, {
		name: "validates metadata",
		s: &Stream{
			ObjectMeta: metav1.ObjectMeta{},
			Spec: StreamSpec{
				Provider:    "kafka",
				ContentType: "audio/vorbis",
			},
		},
		want: apis.ErrMissingOneOf("metadata.generateName", "metadata.name"),
	}, {
		name: "validates spec",
		s: &Stream{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-function",
			},
			Spec: StreamSpec{},
		},
		want: apis.ErrMissingField("spec"),
	}} {
		name := c.name
		t.Run(name, func(t *testing.T) {
			got := c.s.Validate(context.Background())
			if diff := cmp.Diff(c.want.Error(), got.Error()); diff != "" {
				t.Errorf("validateStream(%s) (-want, +got) = %v", name, diff)
			}
		})
	}
}

func TestValidateStreamSpec(t *testing.T) {
	for _, c := range []struct {
		name string
		s    *StreamSpec
		want *apis.FieldError
	}{{
		name: "valid",
		s: &StreamSpec{
			Provider:    "kafka",
			ContentType: "video/mp4",
		},
		want: nil,
	}, {
		name: "valid without explicit content-type",
		s: &StreamSpec{
			Provider: "kafka",
		},
		want: nil,
	}, {
		name: "empty",
		s:    &StreamSpec{},
		want: apis.ErrMissingField(apis.CurrentField),
	}, {
		name: "requires provider",
		s: &StreamSpec{
			Provider:    "",
			ContentType: "image/*",
		},
		want: apis.ErrMissingField("provider"),
	}} {
		name := c.name
		t.Run(name, func(t *testing.T) {
			got := c.s.Validate(context.Background())
			if diff := cmp.Diff(c.want.Error(), got.Error()); diff != "" {
				t.Errorf("validateStreamSpec(%s) (-want, +got) = %v", name, diff)
			}
		})
	}
}
