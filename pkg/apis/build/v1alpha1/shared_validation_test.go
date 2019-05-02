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
)

func TestValidateBuildArgument(t *testing.T) {
	for _, c := range []struct {
		name string
		arg  *BuildArgument
		want *apis.FieldError
	}{{
		name: "valid",
		arg: &BuildArgument{
			Name:  "ARGUMENT_NAME",
			Value: "argument value",
		},
		want: nil,
	}, {
		name: "empty",
		arg:  &BuildArgument{},
		want: apis.ErrMissingField(apis.CurrentField),
	}, {
		name: "requires name",
		arg: &BuildArgument{
			Value: "argument value",
		},
		want: apis.ErrMissingField("name"),
	}, {
		name: "allows empty value",
		arg: &BuildArgument{
			Name:  "ARGUMENT_NAME",
			Value: "",
		},
		want: nil,
	}} {
		name := c.name
		t.Run(name, func(t *testing.T) {
			got := c.arg.Validate(context.Background())
			if diff := cmp.Diff(c.want.Error(), got.Error()); diff != "" {
				t.Errorf("validateBuildArgument(%s) (-want, +got) = %v", name, diff)
			}
		})
	}
}

func TestValidateSource(t *testing.T) {
	for _, c := range []struct {
		name   string
		source *Source
		want   *apis.FieldError
	}{{
		name: "valid",
		source: &Source{
			Git: &GitSource{
				URL:      "https://example.com/repo.git",
				Revision: "master",
			},
		},
		want: nil,
	}, {
		name:   "empty",
		source: &Source{},
		want:   apis.ErrMissingField(apis.CurrentField),
	}, {
		name: "requires a source location",
		source: &Source{
			SubPath: ".",
		},
		want: apis.ErrMissingOneOf("git"),
	}, {
		name: "validates git source",
		source: &Source{
			Git: &GitSource{},
		},
		want: apis.ErrMissingField("git"),
	}} {
		name := c.name
		t.Run(name, func(t *testing.T) {
			got := c.source.Validate(context.Background())
			if diff := cmp.Diff(c.want.Error(), got.Error()); diff != "" {
				t.Errorf("validateSource(%s) (-want, +got) = %v", name, diff)
			}
		})
	}
}

func TestValidateGitSource(t *testing.T) {
	for _, c := range []struct {
		name   string
		source *GitSource
		want   *apis.FieldError
	}{{
		name: "valid",
		source: &GitSource{
			URL:      "https://example.com/repo.git",
			Revision: "master",
		},
		want: nil,
	}, {
		name:   "empty",
		source: &GitSource{},
		want:   apis.ErrMissingField(apis.CurrentField),
	}, {
		name: "requires url",
		source: &GitSource{
			Revision: "master",
		},
		want: apis.ErrMissingField("url"),
	}, {
		name: "requires revision",
		source: &GitSource{
			URL: "https://example.com/repo.git",
		},
		want: apis.ErrMissingField("revision"),
	}} {
		name := c.name
		t.Run(name, func(t *testing.T) {
			got := c.source.Validate(context.Background())
			if diff := cmp.Diff(c.want.Error(), got.Error()); diff != "" {
				t.Errorf("validateGitSource(%s) (-want, +got) = %v", name, diff)
			}
		})
	}
}
