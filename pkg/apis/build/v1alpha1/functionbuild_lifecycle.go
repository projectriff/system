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

	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

const (
	FunctionBuildConditionSucceeded                                  = duckv1alpha1.ConditionSucceeded
	FunctionBuildConditionBuildCacheReady duckv1alpha1.ConditionType = "BuildCacheReady"
	FunctionBuildConditionBuildSucceeded  duckv1alpha1.ConditionType = "BuildSucceeded"
)

var functionbuildCondSet = duckv1alpha1.NewBatchConditionSet(
	FunctionBuildConditionBuildCacheReady,
	FunctionBuildConditionBuildSucceeded,
)

func (ibs *FunctionBuildStatus) IsReady() bool {
	return functionbuildCondSet.Manage(ibs).IsHappy()
}

func (ibs *FunctionBuildStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return functionbuildCondSet.Manage(ibs).GetCondition(t)
}

func (ibs *FunctionBuildStatus) InitializeConditions() {
	functionbuildCondSet.Manage(ibs).InitializeConditions()
}

func (ibs *FunctionBuildStatus) MarkBuildCacheNotOwned(name string) {
	functionbuildCondSet.Manage(ibs).MarkFalse(FunctionBuildConditionBuildCacheReady, "NotOwned",
		fmt.Sprintf("There is an existing PersistentVolumeClaim %q that we do not own.", name))
}

func (ibs *FunctionBuildStatus) MarkBuildCacheNotUsed() {
	ibs.BuildCacheName = ""
	functionbuildCondSet.Manage(ibs).MarkTrue(FunctionBuildConditionBuildCacheReady)
}

func (ibs *FunctionBuildStatus) MarkBuildNotOwned(name string) {
	functionbuildCondSet.Manage(ibs).MarkFalse(FunctionBuildConditionBuildSucceeded, "NotOwned",
		fmt.Sprintf("There is an existing Build %q that we do not own.", name))
}

func (ibs *FunctionBuildStatus) MarkImageMissing(message string) {
	functionbuildCondSet.Manage(ibs).MarkFalse(FunctionBuildConditionBuildSucceeded, "ImageMissing", message)
}

func (ibs *FunctionBuildStatus) PropagateBuildCacheStatus(pvcs *corev1.PersistentVolumeClaimStatus) {
	switch pvcs.Phase {
	case corev1.ClaimPending:
		// used for PersistentVolumeClaims that are not yet bound
		functionbuildCondSet.Manage(ibs).MarkUnknown(FunctionBuildConditionBuildCacheReady, string(pvcs.Phase), "volume claim is not yet bound")
	case corev1.ClaimBound:
		// used for PersistentVolumeClaims that are bound
		functionbuildCondSet.Manage(ibs).MarkTrue(FunctionBuildConditionBuildCacheReady)
	case corev1.ClaimLost:
		// used for PersistentVolumeClaims that lost their underlying PersistentVolume
		functionbuildCondSet.Manage(ibs).MarkFalse(FunctionBuildConditionBuildCacheReady, string(pvcs.Phase), "volume claim lost its underlying volume")
	}
}

func (ibs *FunctionBuildStatus) PropagateBuildStatus(bs *buildv1alpha1.BuildStatus) {
	sc := bs.GetCondition(buildv1alpha1.BuildSucceeded)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		functionbuildCondSet.Manage(ibs).MarkUnknown(FunctionBuildConditionBuildSucceeded, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		functionbuildCondSet.Manage(ibs).MarkTrue(FunctionBuildConditionBuildSucceeded)
	case sc.Status == corev1.ConditionFalse:
		functionbuildCondSet.Manage(ibs).MarkFalse(FunctionBuildConditionBuildSucceeded, sc.Reason, sc.Message)
	}
}
