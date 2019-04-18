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
	"fmt"

	"github.com/knative/pkg/apis"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	"github.com/knative/pkg/kmeta"
	commonv1alpha1 "github.com/projectriff/system/pkg/apis/common/v1alpha1"
	planv1alpha1 "github.com/projectriff/system/pkg/apis/plan/v1alpha1"
	corev1 "k8s.io/api/core/v1"
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
	_ apis.Validatable   = (*Function)(nil)
	_ apis.Defaultable   = (*Function)(nil)
	_ kmeta.OwnerRefable = (*Function)(nil)
)

type FunctionSpec struct {
	Image string              `json:"image"`
	Build *FunctionBuild      `json:"build,omitempty"`
	Run   *commonv1alpha1.Run `json:"run,omitempty"`
}

type FunctionBuild struct {
	Artifact  string                 `json:"artifact,omitempty"`
	Handler   string                 `json:"handler,omitempty"`
	Invoker   string                 `json:"invoker,omitempty"`
	CacheSize *resource.Quantity     `json:"cacheSize,omitempty"`
	Source    *commonv1alpha1.Source `json:"source,omitempty"`
}

const (
	FunctionConditionReady                                = duckv1alpha1.ConditionReady
	FunctionConditionPlanReady duckv1alpha1.ConditionType = "PlanReady"
)

var functionCondSet = duckv1alpha1.NewLivingConditionSet(FunctionConditionPlanReady)

type FunctionStatus struct {
	duckv1alpha1.Status `json:",inline"`
	Address             *duckv1alpha1.Addressable `json:"address,omitempty"`
	Domain              string                    `json:"domain,omitempty"`
	PlanName            string                    `json:"domain,omitempty"`
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

func (fs *FunctionStatus) IsReady() bool {
	return functionCondSet.Manage(fs).IsHappy()
}

func (fs *FunctionStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return functionCondSet.Manage(fs).GetCondition(t)
}

func (fs *FunctionStatus) InitializeConditions() {
	functionCondSet.Manage(fs).InitializeConditions()
}

// TODO move into function reconciler
func (fs *FunctionStatus) MarkPlanNotOwned(name string) {
	functionCondSet.Manage(fs).MarkFalse(FunctionConditionPlanReady, "NotOwned",
		fmt.Sprintf("There is an existing Plan %q that we do not own.", name))
}

// TODO move into function reconciler
func (fs *FunctionStatus) PropagatePlanStatus(p *planv1alpha1.Plan) {
	ps := p.Status

	fs.Address = ps.Address
	fs.Domain = ps.Domain
	fs.PlanName = p.Name

	sc := ps.GetCondition(planv1alpha1.PlanConditionReady)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		functionCondSet.Manage(fs).MarkUnknown(FunctionConditionPlanReady, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		functionCondSet.Manage(fs).MarkTrue(FunctionConditionPlanReady)
	case sc.Status == corev1.ConditionFalse:
		functionCondSet.Manage(fs).MarkFalse(FunctionConditionPlanReady, sc.Reason, sc.Message)
	}
}
