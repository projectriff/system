/*
Copyright 2020 the original author or authors.

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
	"github.com/projectriff/reconciler-runtime/apis"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +genclient

type ServiceBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceBindingSpec   `json:"spec"`
	Status ServiceBindingStatus `json:"status"`
}

type ServiceBindingSpec struct {
	Subject   *Reference                  `json:"subject,omitempty"`
	Providers []ServiceCredentialProvider `json:"providers,omitempty"`
}

type ServiceCredentialProvider struct {
	Ref           ServiceCredentialReference `json:"ref,omitempty"`
	Name          string                     `json:"name"`
	ContainerName string                     `json:"containerName,omitempty"`
	BindingMode   ServiceBindingMode         `json:"bindingMode,omitempty"`
}

type ServiceCredentialReference struct {
	Metadata corev1.LocalObjectReference `json:"metadata"`
	Secret   corev1.LocalObjectReference `json:"secret"`
}

type ServiceBindingMode string

const (
	MetadataServiceBinding ServiceBindingMode = "Metadata"
	SecretServiceBinding   ServiceBindingMode = "Secret"
)

type ServiceBindingStatus struct {
	apis.Status `json:",inline"`
}

// +kubebuilder:object:root=true

type ServiceBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ServiceBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceBinding{}, &ServiceBindingList{})
}
