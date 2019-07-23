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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Function struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FunctionSpec   `json:"spec"`
	Status FunctionStatus `json:"status"`
}

var (
	_ knapis.Validatable = (*Function)(nil)
	_ knapis.Defaultable = (*Function)(nil)
	_ kmeta.OwnerRefable = (*Function)(nil)
	_ apis.Object        = (*Function)(nil)
	_ ImageResource      = (*Function)(nil)
)

type FunctionSpec struct {
	// Image repository to push built images. May contain a leading underscore
	// to have the default image prefix applied.
	Image string `json:"image"`

	// CacheSize of persistent volume to store resources between builds
	CacheSize *resource.Quantity `json:"cacheSize,omitempty"`

	// Source location. Required for on cluster builds.
	Source *Source `json:"source,omitempty"`

	// Artifact file containing the function within the build workspace.
	Artifact string `json:"artifact,omitempty"`

	// Handler name of the method or class to invoke. The value depends on the
	// invoker.
	Handler string `json:"handler,omitempty"`

	// Invoker language runtime name. Detected by default.
	Invoker string `json:"invoker,omitempty"`
}

type FunctionStatus struct {
	duckv1beta1.Status `json:",inline"`
	BuildStatus        `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type FunctionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Function `json:"items"`
}

func (*Function) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Function")
}

func (f *Function) GetStatus() apis.Status {
	return &f.Status
}

func (f *Function) GetImage() string {
	return f.Spec.Image
}
