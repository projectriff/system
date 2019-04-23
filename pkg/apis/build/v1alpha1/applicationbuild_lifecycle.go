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
	ApplicationBuildConditionSucceeded                                  = duckv1alpha1.ConditionSucceeded
	ApplicationBuildConditionBuildCacheReady duckv1alpha1.ConditionType = "BuildCacheReady"
	ApplicationBuildConditionBuildSucceeded  duckv1alpha1.ConditionType = "BuildSucceeded"
)

var applicationbuildCondSet = duckv1alpha1.NewBatchConditionSet(
	ApplicationBuildConditionBuildCacheReady,
	ApplicationBuildConditionBuildSucceeded,
)

func (ibs *ApplicationBuildStatus) IsReady() bool {
	return applicationbuildCondSet.Manage(ibs).IsHappy()
}

func (ibs *ApplicationBuildStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return applicationbuildCondSet.Manage(ibs).GetCondition(t)
}

func (ibs *ApplicationBuildStatus) InitializeConditions() {
	applicationbuildCondSet.Manage(ibs).InitializeConditions()
}

func (ibs *ApplicationBuildStatus) MarkBuildCacheNotOwned(name string) {
	applicationbuildCondSet.Manage(ibs).MarkFalse(ApplicationBuildConditionBuildCacheReady, "NotOwned",
		fmt.Sprintf("There is an existing PersistentVolumeClaim %q that we do not own.", name))
}

func (ibs *ApplicationBuildStatus) MarkBuildCacheNotUsed() {
	ibs.BuildCacheName = ""
	applicationbuildCondSet.Manage(ibs).MarkTrue(ApplicationBuildConditionBuildCacheReady)
}

func (ibs *ApplicationBuildStatus) MarkBuildNotOwned(name string) {
	applicationbuildCondSet.Manage(ibs).MarkFalse(ApplicationBuildConditionBuildSucceeded, "NotOwned",
		fmt.Sprintf("There is an existing Build %q that we do not own.", name))
}

func (ibs *ApplicationBuildStatus) MarkImageMissing(message string) {
	applicationbuildCondSet.Manage(ibs).MarkFalse(ApplicationBuildConditionBuildSucceeded, "ImageMissing", message)
}

func (ibs *ApplicationBuildStatus) PropagateBuildCacheStatus(pvcs *corev1.PersistentVolumeClaimStatus) {
	switch pvcs.Phase {
	case corev1.ClaimPending:
		// used for PersistentVolumeClaims that are not yet bound
		applicationbuildCondSet.Manage(ibs).MarkUnknown(ApplicationBuildConditionBuildCacheReady, string(pvcs.Phase), "volume claim is not yet bound")
	case corev1.ClaimBound:
		// used for PersistentVolumeClaims that are bound
		applicationbuildCondSet.Manage(ibs).MarkTrue(ApplicationBuildConditionBuildCacheReady)
	case corev1.ClaimLost:
		// used for PersistentVolumeClaims that lost their underlying PersistentVolume
		applicationbuildCondSet.Manage(ibs).MarkFalse(ApplicationBuildConditionBuildCacheReady, string(pvcs.Phase), "volume claim lost its underlying volume")
	}
}

func (ibs *ApplicationBuildStatus) PropagateBuildStatus(bs *buildv1alpha1.BuildStatus) {
	sc := bs.GetCondition(buildv1alpha1.BuildSucceeded)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		applicationbuildCondSet.Manage(ibs).MarkUnknown(ApplicationBuildConditionBuildSucceeded, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		applicationbuildCondSet.Manage(ibs).MarkTrue(ApplicationBuildConditionBuildSucceeded)
	case sc.Status == corev1.ConditionFalse:
		applicationbuildCondSet.Manage(ibs).MarkFalse(ApplicationBuildConditionBuildSucceeded, sc.Reason, sc.Message)
	}
}
