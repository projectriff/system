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

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/projectriff/system/pkg/validation"
)

func TestValidateKafkaProvider(t *testing.T) {
	for _, c := range []struct {
		name     string
		target   *KafkaProvider
		expected validation.FieldErrors
	}{{
		name:     "empty",
		target:   &KafkaProvider{},
		expected: validation.ErrMissingField("spec"),
	}, {
		name: "valid",
		target: &KafkaProvider{
			Spec: KafkaProviderSpec{
				BootstrapServers: "localhost:9092",
			},
		},
		expected: validation.FieldErrors{},
	}} {
		t.Run(c.name, func(t *testing.T) {
			actual := c.target.Validate()
			if diff := cmp.Diff(c.expected, actual); diff != "" {
				t.Errorf("validateKafkaProvider(%s) (-expected, +actual) = %v", c.name, diff)
			}
		})
	}
}

func TestValidateKafkaProviderSpec(t *testing.T) {
	for _, c := range []struct {
		name     string
		target   *KafkaProviderSpec
		expected validation.FieldErrors
	}{{
		name:     "empty",
		target:   &KafkaProviderSpec{},
		expected: validation.ErrMissingField(validation.CurrentField),
	}, {
		name: "valid",
		target: &KafkaProviderSpec{
			BootstrapServers: "localhost:9092",
		},
		expected: validation.FieldErrors{},
	}} {
		t.Run(c.name, func(t *testing.T) {
			actual := c.target.Validate()
			if diff := cmp.Diff(c.expected, actual); diff != "" {
				t.Errorf("validateKafkaProviderSpec(%s) (-expected, +actual) = %v", c.name, diff)
			}
		})
	}
}
