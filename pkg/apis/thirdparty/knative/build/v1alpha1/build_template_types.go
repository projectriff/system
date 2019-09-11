/*
Copyright 2019 The Knative Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BuildTemplateSpec is the spec for a BuildTemplate.
type BuildTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// TODO(dprotaso) Metadata.Generation should increment so we
	// can drop this property when conversion webhooks enable us
	// to migrate
	// +optional
	DeprecatedGeneration int64 `json:"generation,omitempty"`

	// Parameters defines the parameters that can be populated in a template.
	Parameters []ParameterSpec `json:"parameters,omitempty"`

	// Steps are the steps of the build; each step is run sequentially with the
	// source mounted into /workspace.
	Steps []corev1.Container `json:"steps"`

	// Volumes is a collection of volumes that are available to mount into the
	// steps of the build.
	Volumes []corev1.Volume `json:"volumes"`
}

// ParameterSpec defines the possible parameters that can be populated in a
// template.
type ParameterSpec struct {
	// Name is the unique name of this template parameter.
	Name string `json:"name"`

	// Description is a human-readable explanation of this template parameter.
	Description string `json:"description,omitempty"`

	// Default, if specified, defines the default value that should be applied if
	// the build does not specify the value for this parameter.
	Default *string `json:"default,omitempty"`
}

// +kubebuilder:object:root=true

// BuildTemplate is a template that can used to easily create Builds.
type BuildTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec BuildTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// BuildTemplateList contains a list of BuildTemplate
type BuildTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BuildTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BuildTemplate{}, &BuildTemplateList{})
}
