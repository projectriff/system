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
	KafkaProviderLabelKey            = GroupVersion.Group + "/kafka-provider"             // Identifies all resources originating from a provider
	KafkaProviderLiiklusLabelKey     = GroupVersion.Group + "/kafka-provider-liiklus"     // Used as a selector
	KafkaProviderProvisionerLabelKey = GroupVersion.Group + "/kafka-provider-provisioner" // Used as a selector
)

var (
	_ apis.Resource = (*KafkaProvider)(nil)
)

// KafkaProviderSpec defines the desired state of KafkaProvider
type KafkaProviderSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// BootstrapServers is a comma-separated list of host and port pairs that are the
	// addresses of the Kafka brokers in a "bootstrap" Kafka cluster that a Kafka client
	// connects to initially to bootstrap itself.
	//
	// A host and port pair uses `:` as the separator.
	BootstrapServers string `json:"bootstrapServers"`
}

// KafkaProviderStatus defines the observed state of KafkaProvider
type KafkaProviderStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	apis.Status               `json:",inline"`
	LiiklusDeploymentName     string `json:"liiklusDeploymentName"`
	LiiklusServiceName        string `json:"liiklusServiceName"`
	ProvisionerDeploymentName string `json:"provisionerDeploymentName"`
	ProvisionerServiceName    string `json:"provisionerServiceName"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories="riff"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +genclient

// KafkaProvider is the Schema for the providers API
type KafkaProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KafkaProviderSpec   `json:"spec,omitempty"`
	Status KafkaProviderStatus `json:"status,omitempty"`
}

func (*KafkaProvider) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("KafkaProvider")
}

func (p *KafkaProvider) GetStatus() apis.ResourceStatus {
	return &p.Status
}

// +kubebuilder:object:root=true

// KafkaProviderList contains a list of KafkaProvider
type KafkaProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KafkaProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KafkaProvider{}, &KafkaProviderList{})
}
