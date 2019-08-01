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
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	"github.com/knative/pkg/kmeta"
	"github.com/projectriff/system/pkg/apis"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Adapter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AdapterSpec   `json:"spec"`
	Status AdapterStatus `json:"status"`
}

var (
	_ knapis.Validatable = (*Adapter)(nil)
	_ knapis.Defaultable = (*Adapter)(nil)
	_ kmeta.OwnerRefable = (*Adapter)(nil)
	_ apis.Object        = (*Adapter)(nil)
)

type AdapterSpec struct {
	// Build resolves the image from a build resource. As the target build
	// produces new images, they will be automatically rolled out to the
	// handler.
	Build Build `json:"build"`

	// Target Knative resource
	Target Target `json:"target"`
}

type Target struct {
	// ServiceRef references a Knative Service in this namespace.
	ServiceRef string `json:"serviceRef,omitempty"`

	// ConfigurationRef references a Knative Configuration in this namespace.
	ConfigurationRef string `json:"configurationRef,omitempty"`
}

type AdapterStatus struct {
	duckv1beta1.Status `json:",inline"`

	// LatestImage is the most recent image resolved from the build and applied
	// to the target
	LatestImage string `json:"latestImage,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AdapterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Adapter `json:"items"`
}

func (*Adapter) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Adapter")
}

func (a *Adapter) GetStatus() apis.Status {
	return &a.Status
}
