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
	commonv1alpha1 "github.com/projectriff/system/pkg/apis/common/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec   `json:"spec"`
	Status ApplicationStatus `json:"status"`
}

var (
	_ apis.Validatable   = (*Application)(nil)
	_ apis.Defaultable   = (*Application)(nil)
	_ kmeta.OwnerRefable = (*Application)(nil)
)

type ApplicationSpec struct {
	Image string              `json:"image"`
	Build *ApplicationBuild   `json:"build,omitempty"`
	Run   *commonv1alpha1.Run `json:"run,omitempty"`
}

type ApplicationBuild struct {
	CacheSize *resource.Quantity     `json:"cacheSize,omitempty"`
	Source    *commonv1alpha1.Source `json:"source,omitempty"`
}

const (
	ApplicationConditionReady                                = duckv1alpha1.ConditionReady
	ApplicationConditionPlanReady duckv1alpha1.ConditionType = "PlanReady"
)

type ApplicationStatus struct {
	duckv1alpha1.Status `json:",inline"`
	Address             *duckv1alpha1.Addressable `json:"address,omitempty"`
	Domain              string                    `json:"domain,omitempty"`
	PlanName            string                    `json:"planName"`
	LatestImage         string                    `json:"latestImage,omitempty"`
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
