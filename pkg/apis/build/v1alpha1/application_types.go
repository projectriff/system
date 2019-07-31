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

// Application builds from source using application buildpacks
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec   `json:"spec"`
	Status ApplicationStatus `json:"status"`
}

var (
	_ knapis.Validatable = (*Application)(nil)
	_ knapis.Defaultable = (*Application)(nil)
	_ kmeta.OwnerRefable = (*Application)(nil)
	_ apis.Object        = (*Application)(nil)
	_ ImageResource      = (*Application)(nil)
)

type ApplicationSpec struct {
	// Image repository to push built images. May contain a leading underscore
	// to have the default image prefix applied, or be `_` to combine the default
	// image prefix with the resource's name as a default value.
	Image string `json:"image"`

	// CacheSize for an optional PersistentVolumeClaim used to store
	// intermediate build artifacts, like a maven cache, for future builds.
	CacheSize *resource.Quantity `json:"cacheSize,omitempty"`

	// Source location. Required for on cluster builds.
	Source *Source `json:"source,omitempty"`
}

type ApplicationStatus struct {
	duckv1beta1.Status `json:",inline"`
	BuildStatus        `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Application `json:"items"`
}

func (*Application) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Application")
}

func (a *Application) GetStatus() apis.Status {
	return &a.Status
}

func (a *Application) GetImage() string {
	return a.Spec.Image
}
