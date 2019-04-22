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
	corev1 "k8s.io/api/core/v1"
)

const (
	ApplicationBuildConditionSucceeded                                         = duckv1alpha1.ConditionSucceeded
	ApplicationBuildConditionInternalBuildSucceeded duckv1alpha1.ConditionType = "InternalBuildSucceeded"
)

var applicationbuildCondSet = duckv1alpha1.NewBatchConditionSet(
	ApplicationBuildConditionInternalBuildSucceeded,
)

func (abs *ApplicationBuildStatus) IsReady() bool {
	return applicationbuildCondSet.Manage(abs).IsHappy()
}

func (abs *ApplicationBuildStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return applicationbuildCondSet.Manage(abs).GetCondition(t)
}

func (abs *ApplicationBuildStatus) InitializeConditions() {
	applicationbuildCondSet.Manage(abs).InitializeConditions()
}

func (abs *ApplicationBuildStatus) MarkInternalBuildNotOwned(name string) {
	applicationbuildCondSet.Manage(abs).MarkFalse(ApplicationBuildConditionInternalBuildSucceeded, "NotOwned",
		fmt.Sprintf("There is an existing InternalBuild %q that we do not own.", name))
}

func (abs *ApplicationBuildStatus) PropagateInternalBuildStatus(ibs *InternalBuildStatus) {
	abs.LatestImage = ibs.LatestImage

	sc := ibs.GetCondition(InternalBuildConditionSucceeded)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		applicationbuildCondSet.Manage(abs).MarkUnknown(ApplicationBuildConditionInternalBuildSucceeded, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		applicationbuildCondSet.Manage(abs).MarkTrue(ApplicationBuildConditionInternalBuildSucceeded)
	case sc.Status == corev1.ConditionFalse:
		applicationbuildCondSet.Manage(abs).MarkFalse(ApplicationBuildConditionInternalBuildSucceeded, sc.Reason, sc.Message)
	}
}
