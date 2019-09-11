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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +kubebuilder:object:root=true
// +genclient:nonNamespaced

// ClusterBuildTemplate is a template that can used to easily create Builds.
type ClusterBuildTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec BuildTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
// +genclient:nonNamespaced

// ClusterBuildTemplateList contains a list of ClusterBuildTemplate
type ClusterBuildTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterBuildTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterBuildTemplate{}, &ClusterBuildTemplateList{})
}
