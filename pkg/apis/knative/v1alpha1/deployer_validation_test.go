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
	corev1 "k8s.io/api/core/v1"

	"github.com/projectriff/system/pkg/validation"
)

func TestValidateDeployer(t *testing.T) {
	for _, c := range []struct {
		name     string
		target   *Deployer
		expected validation.FieldErrors
	}{{
		name:     "empty",
		target:   &Deployer{},
		expected: validation.ErrMissingField("spec"),
	}, {
		name: "valid",
		target: &Deployer{
			Spec: DeployerSpec{
				Build: &Build{
					FunctionRef: "my-function",
				},
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{
						{},
					},
				},
			},
		},
		expected: validation.FieldErrors{},
	}} {
		t.Run(c.name, func(t *testing.T) {
			actual := c.target.Validate()
			if diff := cmp.Diff(c.expected, actual); diff != "" {
				t.Errorf("validateDeployer(%s) (-expected, +actual) = %v", c.name, diff)
			}
		})
	}
}

func TestValidateDeployerSpec(t *testing.T) {
	negativeOne := int64(-1)
	zero := int64(0)
	one := int64(1)
	five := int64(5)

	for _, c := range []struct {
		name     string
		target   *DeployerSpec
		expected validation.FieldErrors
	}{{
		name:     "empty",
		target:   &DeployerSpec{},
		expected: validation.ErrMissingField(validation.CurrentField),
	}, {
		name: "valid, build",
		target: &DeployerSpec{
			Build: &Build{
				FunctionRef: "my-function",
			},
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{
					{},
				},
			},
		},
		expected: validation.FieldErrors{},
	}, {
		name: "valid, container image",
		target: &DeployerSpec{
			Build: nil,
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Image: "my-image"},
				},
			},
		},
		expected: validation.FieldErrors{},
	}, {
		name: "invalid, build and container image",
		target: &DeployerSpec{
			Build: &Build{
				FunctionRef: "my-function",
			},
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Image: "my-image"},
				},
			},
		},
		expected: validation.ErrMultipleOneOf("build", "template.containers[0].image"),
	}, {
		name: "invalid, ingress policy",
		target: &DeployerSpec{
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Image: "my-image"},
				},
			},
			IngressPolicy: "bogus",
		},
		expected: validation.ErrInvalidValue(IngressPolicy("bogus"), "ingressPolicy"),
	}, {
		name: "valid, empty scale",
		target: &DeployerSpec{
			Build: nil,
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Image: "my-image"},
				},
			},
			Scale: &Scale{},
		},
		expected: validation.FieldErrors{},
	}, {
		name: "valid, minScale",
		target: &DeployerSpec{
			Build: nil,
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Image: "my-image"},
				},
			},
			Scale: &Scale{
				Minimum: &one,
			},
		},
		expected: validation.FieldErrors{},
	}, {
		name: "invalid, negative minScale",
		target: &DeployerSpec{
			Build: nil,
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Image: "my-image"},
				},
			},
			Scale: &Scale{
				Minimum: &negativeOne,
			},
		},
		expected: validation.ErrInvalidValue(negativeOne, "scale.minimum"),
	}, {
		name: "valid, maxScale",
		target: &DeployerSpec{
			Build: nil,
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Image: "my-image"},
				},
			},
			Scale: &Scale{
				Maximum: &one,
			},
		},
		expected: validation.FieldErrors{},
	}, {
		name: "invalid, non-positive maxScale",
		target: &DeployerSpec{
			Build: nil,
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Image: "my-image"},
				},
			},
			Scale: &Scale{
				Maximum: &zero,
			},
		},
		expected: validation.ErrInvalidValue(zero, "scale.maximum"),
	}, {
		name: "valid, minScale and maxScale",
		target: &DeployerSpec{
			Build: nil,
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Image: "my-image"},
				},
			},
			Scale: &Scale{
				Minimum: &one,
				Maximum: &five,
			},
		},
		expected: validation.FieldErrors{},
	}, {
		name: "invalid, maxScale lower than minScale",
		target: &DeployerSpec{
			Build: nil,
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Image: "my-image"},
				},
			},
			Scale: &Scale{
				Minimum: &five,
				Maximum: &one,
			},
		},
		expected: validation.ErrInvalidValue(one, "scale.maximum"),
	}} {
		t.Run(c.name, func(t *testing.T) {
			actual := c.target.Validate()
			if diff := cmp.Diff(c.expected, actual); diff != "" {
				t.Errorf("validateDeployerSpec(%s) (-expected, +actual) = %v", c.name, diff)
			}
		})
	}
}
