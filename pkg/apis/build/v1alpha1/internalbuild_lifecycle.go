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
	InternalBuildConditionSucceeded                                  = duckv1alpha1.ConditionSucceeded
	InternalBuildConditionBuildCacheReady duckv1alpha1.ConditionType = "BuildCacheReady"
	InternalBuildConditionBuildSucceeded  duckv1alpha1.ConditionType = "BuildSucceeded"
)

var internalbuildCondSet = duckv1alpha1.NewBatchConditionSet(
	InternalBuildConditionBuildCacheReady,
	InternalBuildConditionBuildSucceeded,
)

func (ibs *InternalBuildStatus) IsReady() bool {
	return internalbuildCondSet.Manage(ibs).IsHappy()
}

func (ibs *InternalBuildStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return internalbuildCondSet.Manage(ibs).GetCondition(t)
}

func (ibs *InternalBuildStatus) InitializeConditions() {
	internalbuildCondSet.Manage(ibs).InitializeConditions()
}

func (ibs *InternalBuildStatus) MarkBuildCacheNotOwned(name string) {
	internalbuildCondSet.Manage(ibs).MarkFalse(InternalBuildConditionBuildCacheReady, "NotOwned",
		fmt.Sprintf("There is an existing PersistentVolumeClaim %q that we do not own.", name))
}

func (ibs *InternalBuildStatus) MarkBuildCacheNotUsed() {
	ibs.BuildCacheName = ""
	internalbuildCondSet.Manage(ibs).MarkTrue(InternalBuildConditionBuildCacheReady)
}

func (ibs *InternalBuildStatus) MarkBuildNotOwned(name string) {
	internalbuildCondSet.Manage(ibs).MarkFalse(InternalBuildConditionBuildSucceeded, "NotOwned",
		fmt.Sprintf("There is an existing Build %q that we do not own.", name))
}

func (ibs *InternalBuildStatus) MarkImageMissing(message string) {
	internalbuildCondSet.Manage(ibs).MarkFalse(InternalBuildConditionBuildSucceeded, "ImageMissing", message)
}

func (ibs *InternalBuildStatus) PropagateBuildCacheStatus(pvcs *corev1.PersistentVolumeClaimStatus) {
	switch pvcs.Phase {
	case corev1.ClaimPending:
		// used for PersistentVolumeClaims that are not yet bound
		internalbuildCondSet.Manage(ibs).MarkUnknown(InternalBuildConditionBuildCacheReady, string(pvcs.Phase), "volume claim is not yet bound")
	case corev1.ClaimBound:
		// used for PersistentVolumeClaims that are bound
		internalbuildCondSet.Manage(ibs).MarkTrue(InternalBuildConditionBuildCacheReady)
	case corev1.ClaimLost:
		// used for PersistentVolumeClaims that lost their underlying PersistentVolume
		internalbuildCondSet.Manage(ibs).MarkFalse(InternalBuildConditionBuildCacheReady, string(pvcs.Phase), "volume claim lost its underlying volume")
	}
}

func (ibs *InternalBuildStatus) PropagateBuildStatus(bs *buildv1alpha1.BuildStatus) {
	sc := bs.GetCondition(buildv1alpha1.BuildSucceeded)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		internalbuildCondSet.Manage(ibs).MarkUnknown(InternalBuildConditionBuildSucceeded, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		internalbuildCondSet.Manage(ibs).MarkTrue(InternalBuildConditionBuildSucceeded)
	case sc.Status == corev1.ConditionFalse:
		internalbuildCondSet.Manage(ibs).MarkFalse(InternalBuildConditionBuildSucceeded, sc.Reason, sc.Message)
	}
}
