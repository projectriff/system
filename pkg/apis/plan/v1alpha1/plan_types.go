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
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	commonv1alpha1 "github.com/projectriff/system/pkg/apis/common/v1alpha1"
	corev1 "k8s.io/api/core/v1"
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
	_ apis.Validatable   = (*RunPlan)(nil)
	_ apis.Defaultable   = (*RunPlan)(nil)
	_ kmeta.OwnerRefable = (*RunPlan)(nil)
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

var planCondSet = duckv1alpha1.NewLivingConditionSet(
	PlanConditionBuildPlanReady,
	PlanConditionRunPlanReady,
	PlanConditionRouteReady,
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

func (ps *PlanStatus) IsReady() bool {
	return planCondSet.Manage(ps).IsHappy()
}

func (ps *PlanStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return planCondSet.Manage(ps).GetCondition(t)
}

func (ps *PlanStatus) InitializeConditions() {
	planCondSet.Manage(ps).InitializeConditions()
}

// TODO move into workload reconciler
func (ps *PlanStatus) MarkBuildPlanNotOwned(name string) {
	planCondSet.Manage(ps).MarkFalse(PlanConditionBuildPlanReady, "NotOwned",
		fmt.Sprintf("There is an existing BuildPlan %q that we do not own.", name))
}

// TODO move into workload reconciler
func (ps *PlanStatus) MarkRunPlanNotOwned(name string) {
	planCondSet.Manage(ps).MarkFalse(PlanConditionRunPlanReady, "NotOwned",
		fmt.Sprintf("There is an existing RunPlan %q that we do not own.", name))
}

// TODO move into workload reconciler
func (ps *PlanStatus) MarkRouteNotOwned(name string) {
	planCondSet.Manage(ps).MarkFalse(PlanConditionRouteReady, "NotOwned",
		fmt.Sprintf("There is an existing Route %q that we do not own.", name))
}

// TODO move into workload reconciler
func (ps *PlanStatus) PropagateBuildPlanStatus(bp *BuildPlan) {
	if bp == nil {
		// no build is defined
		ps.LatestImage = bp.Spec.Image
		planCondSet.Manage(ps).MarkTrue(PlanConditionBuildPlanReady)
		return
	}
	bps := bp.Status

	if bps.LatestBuiltImage != "" {
		ps.LatestImage = bps.LatestBuiltImage
	}

	rc := bps.GetCondition(BuildPlanConditionReady)
	if rc == nil {
		return
	}
	switch {
	case rc.Status == corev1.ConditionUnknown:
		planCondSet.Manage(ps).MarkUnknown(PlanConditionBuildPlanReady, rc.Reason, rc.Message)
	case rc.Status == corev1.ConditionTrue:
		planCondSet.Manage(ps).MarkTrue(PlanConditionBuildPlanReady)
	case rc.Status == corev1.ConditionFalse:
		planCondSet.Manage(ps).MarkFalse(PlanConditionBuildPlanReady, rc.Reason, rc.Message)
	}
}

// TODO move into workload reconciler
func (ps *PlanStatus) PropagateRunPlanStatus(rp *RunPlan) {
	if rp == nil {
		// no run is defined
		ps.ConfigurationName = ""
		planCondSet.Manage(ps).MarkTrue(PlanConditionRunPlanReady)
		return
	}
	rps := rp.Status

	ps.ConfigurationName = rps.ConfigurationName

	rc := rps.GetCondition(RunPlanConditionReady)
	if rc == nil {
		return
	}
	switch {
	case rc.Status == corev1.ConditionUnknown:
		planCondSet.Manage(ps).MarkUnknown(PlanConditionRunPlanReady, rc.Reason, rc.Message)
	case rc.Status == corev1.ConditionTrue:
		planCondSet.Manage(ps).MarkTrue(PlanConditionRunPlanReady)
	case rc.Status == corev1.ConditionFalse:
		planCondSet.Manage(ps).MarkFalse(PlanConditionRunPlanReady, rc.Reason, rc.Message)
	}
}

// TODO move into workload reconciler
func (ps *PlanStatus) PropagateRouteStatus(r *servingv1alpha1.Route) {
	if r == nil {
		ps.Address = nil
		ps.Domain = ""
		planCondSet.Manage(ps).MarkTrue(PlanConditionRouteReady)
		return
	}
	rs := r.Status

	ps.Address = rs.Address
	ps.Domain = rs.Domain

	rc := rs.GetCondition(servingv1alpha1.RouteConditionReady)
	if rc == nil {
		return
	}
	switch {
	case rc.Status == corev1.ConditionUnknown:
		planCondSet.Manage(ps).MarkUnknown(PlanConditionRouteReady, rc.Reason, rc.Message)
	case rc.Status == corev1.ConditionTrue:
		planCondSet.Manage(ps).MarkTrue(PlanConditionRouteReady)
	case rc.Status == corev1.ConditionFalse:
		planCondSet.Manage(ps).MarkFalse(PlanConditionRouteReady, rc.Reason, rc.Message)
	}
}
