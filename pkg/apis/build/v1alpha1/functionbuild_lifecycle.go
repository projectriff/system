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
	FunctionBuildConditionSucceeded                                         = duckv1alpha1.ConditionSucceeded
	FunctionBuildConditionInternalBuildSucceeded duckv1alpha1.ConditionType = "InternalBuildSucceeded"
)

var functionbuildCondSet = duckv1alpha1.NewBatchConditionSet(FunctionBuildConditionInternalBuildSucceeded)

func (fbs *FunctionBuildStatus) IsReady() bool {
	return functionbuildCondSet.Manage(fbs).IsHappy()
}

func (fbs *FunctionBuildStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return functionbuildCondSet.Manage(fbs).GetCondition(t)
}

func (fbs *FunctionBuildStatus) InitializeConditions() {
	functionbuildCondSet.Manage(fbs).InitializeConditions()
}

func (fbs *FunctionBuildStatus) MarkInternalBuildNotOwned(name string) {
	functionbuildCondSet.Manage(fbs).MarkFalse(FunctionBuildConditionInternalBuildSucceeded, "NotOwned",
		fmt.Sprintf("There is an existing InternalBuild %q that we do not own.", name))
}

func (fbs *FunctionBuildStatus) PropagateInternalBuildStatus(ibs *InternalBuildStatus) {
	fbs.LatestImage = ibs.LatestImage

	sc := ibs.GetCondition(InternalBuildConditionSucceeded)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		functionbuildCondSet.Manage(fbs).MarkUnknown(FunctionBuildConditionInternalBuildSucceeded, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		functionbuildCondSet.Manage(fbs).MarkTrue(FunctionBuildConditionInternalBuildSucceeded)
	case sc.Status == corev1.ConditionFalse:
		functionbuildCondSet.Manage(fbs).MarkFalse(FunctionBuildConditionInternalBuildSucceeded, sc.Reason, sc.Message)
	}
}
