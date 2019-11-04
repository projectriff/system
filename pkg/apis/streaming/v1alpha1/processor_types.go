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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/projectriff/system/pkg/apis"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

var (
	ProcessorLabelKey = GroupVersion.Group + "/processor"
)

var (
	_ apis.Resource = (*Processor)(nil)
)

// ProcessorSpec defines the desired state of Processor
type ProcessorSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// FunctionRef references a function in this namespace.
	FunctionRef string `json:"functionRef"`
	// Inputs references an ordered list of stream names from this namespace.
	Inputs []string `json:"inputs"`
	// Outputs references an ordered list of stream names from this namespace.
	Outputs []string `json:"outputs"`
	// InputNames is a list of argument names, to be used by languages that support that concept.
	InputNames []string `json:"inputNames"`
	// OutputNames is a list of result names, to be used by languages that support that concept.
	OutputNames []string `json:"outputNames"`
}

// ProcessorStatus defines the observed state of Processor
type ProcessorStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	apis.Status `json:",inline"`

	InputAddresses     []string `json:"inputAddresses,omitempty"`
	OutputAddresses    []string `json:"outputAddresses,omitempty"`
	OutputContentTypes []string `json:"outputContentTypes,omitempty"`
	DeploymentName     string   `json:"deploymentName,omitempty"`
	ScaledObjectName   string   `json:"scaledObjectName,omitempty"`
	FunctionImage      string   `json:"functionImage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories="riff"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +genclient

// Processor is the Schema for the processors API
type Processor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProcessorSpec   `json:"spec,omitempty"`
	Status ProcessorStatus `json:"status,omitempty"`
}

func (*Processor) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Processor")
}

func (p *Processor) GetStatus() apis.ResourceStatus {
	return &p.Status
}

// +kubebuilder:object:root=true

// ProcessorList contains a list of Processor
type ProcessorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Processor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Processor{}, &ProcessorList{})
}
