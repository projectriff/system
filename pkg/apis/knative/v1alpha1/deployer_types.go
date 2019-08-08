/*
 * Copyright 2019 The original author or authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1alpha1

import (
	knapis "github.com/knative/pkg/apis"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	"github.com/knative/pkg/kmeta"
	"github.com/projectriff/system/pkg/apis"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Deployer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeployerSpec   `json:"spec"`
	Status DeployerStatus `json:"status"`
}

var (
	_ knapis.Validatable = (*Deployer)(nil)
	_ knapis.Defaultable = (*Deployer)(nil)
	_ kmeta.OwnerRefable = (*Deployer)(nil)
	_ apis.Object        = (*Deployer)(nil)
)

type DeployerSpec struct {
	// Build resolves the image from a build resource. As the target build
	// produces new images, they will be automatically rolled out to the
	// deployer.
	Build *Build `json:"build,omitempty"`

	// Template pod
	Template *corev1.PodSpec `json:"template,omitempty"`
}

type DeployerStatus struct {
	duckv1beta1.Status `json:",inline"`

	// ConfigurationName is the name of the Knative Serving configuration
	// backing this deployer.
	ConfigurationName string `json:"configurationName,omitempty"`

	// RouteName is the name of the Knative Serving route backing this
	// deployer.
	RouteName string `json:"routeName,omitempty"`

	// Address to target this deployer internally
	Address *duckv1alpha1.Addressable `json:"address,omitempty"`

	// URL to target this deployer publicly
	URL *knapis.URL `json:"url,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DeployerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Deployer `json:"items"`
}

func (*Deployer) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Deployer")
}

func (c *Deployer) GetStatus() apis.Status {
	return &c.Status
}
