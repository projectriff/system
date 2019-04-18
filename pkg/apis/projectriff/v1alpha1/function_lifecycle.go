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

	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	planv1alpha1 "github.com/projectriff/system/pkg/apis/plan/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

var functionCondSet = duckv1alpha1.NewLivingConditionSet(FunctionConditionPlanReady)

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
	fs.LatestImage = ps.LatestImage

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
