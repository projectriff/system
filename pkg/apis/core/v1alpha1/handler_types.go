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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Handler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HandlerSpec   `json:"spec"`
	Status HandlerStatus `json:"status"`
}

var (
	_ knapis.Validatable = (*Handler)(nil)
	_ knapis.Defaultable = (*Handler)(nil)
	_ kmeta.OwnerRefable = (*Handler)(nil)
	_ apis.Object        = (*Handler)(nil)
)

type HandlerSpec struct {
	// Build resolves the image from a build resource. As the target build
	// produces new images, they will be automatically rolled out to the
	// handler.
	Build *Build `json:"build,omitempty"`

	// Template pod
	Template *corev1.PodSpec `json:"template,omitempty"`
}

type Build struct {
	// ApplicationRef references an application in this namespace.
	ApplicationRef string `json:"applicationRef,omitempty"`

	// ContainerRef references a container in this namespace.
	ContainerRef string `json:"containerRef,omitempty"`

	// FunctionRef references an application in this namespace.
	FunctionRef string `json:"functionRef,omitempty"`
}

type HandlerStatus struct {
	duckv1beta1.Status `json:",inline"`
	DeploymentName     string `json:"deploymentName,omitempty"`
	ServiceName        string `json:"serviceName,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HandlerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Handler `json:"items"`
}

func (*Handler) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Handler")
}

func (h *Handler) GetStatus() apis.Status {
	return &h.Status
}
