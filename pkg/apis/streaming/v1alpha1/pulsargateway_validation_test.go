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
	"github.com/vmware-labs/reconciler-runtime/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestValidatePulsarGateway(t *testing.T) {
	for _, c := range []struct {
		name     string
		target   *PulsarGateway
		expected validation.FieldErrors
	}{{
		name:     "empty",
		target:   &PulsarGateway{},
		expected: validation.ErrMissingField("spec"),
	}, {
		name: "valid",
		target: &PulsarGateway{
			Spec: PulsarGatewaySpec{
				ServiceURL: "pulsar://localhost:6650",
			},
		},
		expected: validation.FieldErrors{},
	}} {
		t.Run(c.name, func(t *testing.T) {
			actual := c.target.Validate()
			if diff := cmp.Diff(c.expected, actual); diff != "" {
				t.Errorf("validatePulsarGateway(%s) (-expected, +actual) = %v", c.name, diff)
			}
		})
	}
}

func TestValidatePulsarGatewaySpec(t *testing.T) {
	for _, c := range []struct {
		name     string
		target   *PulsarGatewaySpec
		expected validation.FieldErrors
	}{{
		name:     "empty",
		target:   &PulsarGatewaySpec{},
		expected: validation.ErrMissingField(validation.CurrentField),
	}, {
		name: "valid",
		target: &PulsarGatewaySpec{
			ServiceURL: "pulsar://localhost:6650",
		},
		expected: validation.FieldErrors{},
	}, {
		name: "valid+ssl",
		target: &PulsarGatewaySpec{
			ServiceURL: "pulsar+ssl://localhost:6650",
		},
		expected: validation.FieldErrors{},
	}, {
		name: "wrong-scheme",
		target: &PulsarGatewaySpec{
			ServiceURL: "localhost:6650",
		},
		expected: validation.FieldErrors{field.Invalid(field.NewPath("serviceURL"), "localhost:6650", "serviceURL must use 'pulsar://' or 'pulsar+ssl://' scheme")},
	}} {
		t.Run(c.name, func(t *testing.T) {
			actual := c.target.Validate()
			if diff := cmp.Diff(c.expected, actual); diff != "" {
				t.Errorf("validatePulsarGatewaySpec(%s) (-expected, +actual) = %v", c.name, diff)
			}
		})
	}
}
