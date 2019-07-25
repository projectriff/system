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

// Container watches a repository for new images
type Container struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ContainerSpec   `json:"spec"`
	Status ContainerStatus `json:"status"`
}

var (
	_ knapis.Validatable = (*Container)(nil)
	_ knapis.Defaultable = (*Container)(nil)
	_ kmeta.OwnerRefable = (*Container)(nil)
	_ apis.Object        = (*Container)(nil)
	_ ImageResource      = (*Container)(nil)
)

type ContainerSpec struct {
	// Image repository to watch for built images. May contain a leading underscore
	// to have the default image prefix applied.
	Image string `json:"image"`
}

type ContainerStatus struct {
	duckv1beta1.Status `json:",inline"`
	BuildStatus        `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ContainerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Container `json:"items"`
}

func (*Container) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Container")
}

func (a *Container) GetStatus() apis.Status {
	return &a.Status
}

func (a *Container) GetImage() string {
	return a.Spec.Image
}
