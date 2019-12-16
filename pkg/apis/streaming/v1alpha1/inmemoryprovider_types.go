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
	"github.com/projectriff/system/pkg/refs"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

var (
	InMemoryProviderLabelKey            = GroupVersion.Group + "/inmemory-provider"             // Identifies all resources originating from a provider
	InMemoryProviderGatewayLabelKey     = GroupVersion.Group + "/inmemory-provider-gateway"     // Used as a selector
	InMemoryProviderProvisionerLabelKey = GroupVersion.Group + "/inmemory-provider-provisioner" // Used as a selector
	InMemoryProvisioner                 = "inmemory-provisioner"
)

var (
	_ apis.Resource = (*InMemoryProvider)(nil)
)

// InMemoryProviderSpec defines the desired state of InMemoryProvider
type InMemoryProviderSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// InMemoryProviderStatus defines the observed state of InMemoryProvider
type InMemoryProviderStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	apis.Status              `json:",inline"`
	GatewayDeploymentRef     *refs.TypedLocalObjectReference `json:"gatewayDeploymentRef,omitempty"`
	GatewayServiceRef        *refs.TypedLocalObjectReference `json:"gatewayServiceRef,omitempty"`
	ProvisionerDeploymentRef *refs.TypedLocalObjectReference `json:"provisionerDeploymentRef,omitempty"`
	ProvisionerServiceRef    *refs.TypedLocalObjectReference `json:"provisionerServiceRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories="riff"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +genclient

// InMemoryProvider is the Schema for the providers API
type InMemoryProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InMemoryProviderSpec   `json:"spec,omitempty"`
	Status InMemoryProviderStatus `json:"status,omitempty"`
}

func (*InMemoryProvider) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("InMemoryProvider")
}

func (p *InMemoryProvider) GetStatus() apis.ResourceStatus {
	return &p.Status
}

// +kubebuilder:object:root=true

// InMemoryProviderList contains a list of InMemoryProvider
type InMemoryProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InMemoryProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InMemoryProvider{}, &InMemoryProviderList{})
}
