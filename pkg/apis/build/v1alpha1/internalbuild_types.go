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
	"github.com/knative/pkg/apis"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	"github.com/knative/pkg/kmeta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type InternalBuild struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InternalBuildSpec   `json:"spec"`
	Status InternalBuildStatus `json:"status"`
}

var (
	_ apis.Validatable   = (*InternalBuild)(nil)
	_ apis.Defaultable   = (*InternalBuild)(nil)
	_ kmeta.OwnerRefable = (*InternalBuild)(nil)
)

type InternalBuildSpec struct {
	Image     string             `json:"image"`
	CacheSize *resource.Quantity `json:"cacheSize,omitempty"`
	Source    Source             `json:"source,omitempty"`
	Template  string             `json:"template"`
	Arguments []BuildArgument    `json:"arguments,omitempty"`
}

type InternalBuildStatus struct {
	duckv1alpha1.Status `json:",inline"`
	BuildCacheName      string `json:"buildCacheName"`
	BuildName           string `json:"buildName"`
	LatestImage         string `json:"latestImage,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type InternalBuildList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []InternalBuild `json:"items"`
}

func (*InternalBuild) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("InternalBuild")
}
