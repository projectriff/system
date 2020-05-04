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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +genclient

type ImageBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageBindingSpec   `json:"spec"`
	Status ImageBindingStatus `json:"status"`
}

type ImageBindingSpec struct {
	Subject       *Reference `json:"subject,omitempty"`
	Provider      *Reference `json:"provider,omitempty"`
	ContainerName string     `json:"containerName,omitempty"`
}

type ImageBindingStatus struct {
	apis.Status `json:",inline"`
}

// +kubebuilder:object:root=true

type ImageBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ImageBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImageBinding{}, &ImageBindingList{})
}
