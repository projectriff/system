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

var applicationCondSet = duckv1alpha1.NewLivingConditionSet(
	ApplicationConditionPlanReady,
)

func (as *ApplicationStatus) IsReady() bool {
	return applicationCondSet.Manage(as).IsHappy()
}

func (as *ApplicationStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return applicationCondSet.Manage(as).GetCondition(t)
}

func (as *ApplicationStatus) InitializeConditions() {
	applicationCondSet.Manage(as).InitializeConditions()
}

// TODO move into application reconciler
func (as *ApplicationStatus) MarkPlanNotOwned(name string) {
	applicationCondSet.Manage(as).MarkFalse(ApplicationConditionPlanReady, "NotOwned",
		fmt.Sprintf("There is an existing Plan %q that we do not own.", name))
}

// TODO move into application reconciler
func (as *ApplicationStatus) PropagatePlanStatus(p *planv1alpha1.Plan) {
	ps := p.Status

	as.Address = ps.Address
	as.Domain = ps.Domain
	as.PlanName = p.Name
	as.LatestImage = ps.LatestImage

	sc := ps.GetCondition(planv1alpha1.PlanConditionReady)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		applicationCondSet.Manage(as).MarkUnknown(ApplicationConditionPlanReady, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		applicationCondSet.Manage(as).MarkTrue(ApplicationConditionPlanReady)
	case sc.Status == corev1.ConditionFalse:
		applicationCondSet.Manage(as).MarkFalse(ApplicationConditionPlanReady, sc.Reason, sc.Message)
	}
}
