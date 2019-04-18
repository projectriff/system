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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Plan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlanSpec   `json:"spec"`
	Status PlanStatus `json:"status"`
}

var (
	_ apis.Validatable   = (*Plan)(nil)
	_ apis.Defaultable   = (*Plan)(nil)
	_ kmeta.OwnerRefable = (*Plan)(nil)
)

type PlanSpec struct {
	Image string                `json:"image"`
	Build *commonv1alpha1.Build `json:"build,omitempty"`
	Run   *commonv1alpha1.Run   `json:"run"`
}

const (
	PlanConditionReady                                     = duckv1alpha1.ConditionReady
	PlanConditionBuildPlanReady duckv1alpha1.ConditionType = "BuildPlanReady"
	PlanConditionRunPlanReady   duckv1alpha1.ConditionType = "RunPlanReady"
	PlanConditionRouteReady     duckv1alpha1.ConditionType = "RouteReady"
)

type PlanStatus struct {
	duckv1alpha1.Status `json:",inline"`
	Address             *duckv1alpha1.Addressable `json:"address,omitempty"`
	Domain              string                    `json:"domain,omitempty"`
	ConfigurationName   string                    `json:"configurationName,omitempty"`
	LatestImage         string                    `json:"latestImage,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Plan `json:"items"`
}

func (*Plan) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Plan")
}
