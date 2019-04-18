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

type RunPlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RunPlanSpec   `json:"spec"`
	Status RunPlanStatus `json:"status"`
}

var (
	_ apis.Validatable   = (*RunPlan)(nil)
	_ apis.Defaultable   = (*RunPlan)(nil)
	_ kmeta.OwnerRefable = (*RunPlan)(nil)
)

type RunPlanSpec struct {
	Image string             `json:"image"`
	Run   commonv1alpha1.Run `json:",inline"`
}

const (
	RunPlanConditionReady                                         = duckv1alpha1.ConditionReady
	RunPlanConditionConfigurationReady duckv1alpha1.ConditionType = "ConfigurationReady"
)

var runplanCondSet = duckv1alpha1.NewLivingConditionSet(
	RunPlanConditionConfigurationReady,
)

type RunPlanStatus struct {
	duckv1alpha1.Status `json:",inline"`
	ConfigurationName   string `json:"configurationName,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RunPlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []RunPlan `json:"items"`
}

func (*RunPlan) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("RunPlan")
}

func (rps *RunPlanStatus) IsReady() bool {
	return runplanCondSet.Manage(rps).IsHappy()
}

func (rps *RunPlanStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return runplanCondSet.Manage(rps).GetCondition(t)
}

func (rps *RunPlanStatus) InitializeConditions() {
	runplanCondSet.Manage(rps).InitializeConditions()
}

// TODO move into workload reconciler
func (rps *RunPlanStatus) MarkConfigurationNotOwned(name string) {
	runplanCondSet.Manage(rps).MarkFalse(RunPlanConditionConfigurationReady, "NotOwned",
		fmt.Sprintf("There is an existing Configuration %q that we do not own.", name))
}

// TODO move into workload reconciler
func (rps *RunPlanStatus) PropagateConfigurationStatus(c *servingv1alpha1.Configuration) {
	cs := c.Status

	rps.ConfigurationName = c.Name

	rc := cs.GetCondition(servingv1alpha1.ConfigurationConditionReady)
	if rc == nil {
		return
	}
	switch {
	case rc.Status == corev1.ConditionUnknown:
		runplanCondSet.Manage(rps).MarkUnknown(RunPlanConditionConfigurationReady, rc.Reason, rc.Message)
	case rc.Status == corev1.ConditionTrue:
		runplanCondSet.Manage(rps).MarkTrue(RunPlanConditionConfigurationReady)
	case rc.Status == corev1.ConditionFalse:
		runplanCondSet.Manage(rps).MarkFalse(RunPlanConditionConfigurationReady, rc.Reason, rc.Message)
	}
}
