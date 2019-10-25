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
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// +kubebuilder:webhook:path=/mutate-knative-projectriff-io-v1alpha1-deployer,mutating=true,failurePolicy=fail,groups=knative.projectriff.io,resources=deployers,verbs=create;update,versions=v1alpha1,name=deployers.build.projectriff.io

var _ webhook.Defaulter = &Deployer{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Deployer) Default() {
	r.Spec.Default()
}

func (s *DeployerSpec) Default() {
	if s.Template == nil {
		s.Template = &corev1.PodSpec{}
	}
	if len(s.Template.Containers) == 0 {
		s.Template.Containers = append(s.Template.Containers, corev1.Container{})
	}
}
