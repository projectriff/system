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

var buildplanCondSet = duckv1alpha1.NewLivingConditionSet(
	BuildPlanConditionBuildCacheReady,
	BuildPlanConditionBuildSucceeded,
)

func (bps *BuildPlanStatus) IsReady() bool {
	return buildplanCondSet.Manage(bps).IsHappy()
}

func (bps *BuildPlanStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return buildplanCondSet.Manage(bps).GetCondition(t)
}

func (bps *BuildPlanStatus) InitializeConditions() {
	buildplanCondSet.Manage(bps).InitializeConditions()
}

// TODO move into buildplan reconciler
func (bps *BuildPlanStatus) MarkBuildCacheNotOwned(name string) {
	buildplanCondSet.Manage(bps).MarkFalse(BuildPlanConditionBuildCacheReady, "NotOwned",
		fmt.Sprintf("There is an existing PersistentVolumeClaim %q that we do not own.", name))
}

// TODO move into buildplan reconciler
func (bps *BuildPlanStatus) MarkBuildNotOwned(name string) {
	buildplanCondSet.Manage(bps).MarkFalse(BuildPlanConditionBuildSucceeded, "NotOwned",
		fmt.Sprintf("There is an existing Build %q that we do not own.", name))
}

// TODO move into buildplan reconciler
func (bps *BuildPlanStatus) PropagateBuildCacheStatus(pvc *corev1.PersistentVolumeClaim) {
	if pvc == nil {
		bps.BuildCacheName = ""
		buildplanCondSet.Manage(bps).MarkTrue(BuildPlanConditionBuildCacheReady)
		return
	}
	pvcs := pvc.Status

	bps.BuildCacheName = pvc.Name

	switch pvcs.Phase {
	case corev1.ClaimPending:
		// used for PersistentVolumeClaims that are not yet bound
		buildplanCondSet.Manage(bps).MarkUnknown(BuildPlanConditionBuildCacheReady, string(pvcs.Phase), "volume claim is not yet bound")
	case corev1.ClaimBound:
		// used for PersistentVolumeClaims that are bound
		buildplanCondSet.Manage(bps).MarkTrue(BuildPlanConditionBuildCacheReady)
	case corev1.ClaimLost:
		// used for PersistentVolumeClaims that lost their underlying PersistentVolume
		buildplanCondSet.Manage(bps).MarkFalse(BuildPlanConditionBuildCacheReady, string(pvcs.Phase), "volume claim lost its underlying volume")
	}
}

// TODO move into buildplan reconciler
func (bps *BuildPlanStatus) PropagateBuildStatus(b *buildv1alpha1.Build) {
	if b == nil {
		bps.BuildName = ""
		buildplanCondSet.Manage(bps).MarkTrue(BuildPlanConditionBuildSucceeded)
		return
	}
	bs := b.Status

	bps.BuildName = b.Name

	sc := bs.GetCondition(buildv1alpha1.BuildSucceeded)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		buildplanCondSet.Manage(bps).MarkUnknown(BuildPlanConditionBuildSucceeded, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		buildplanCondSet.Manage(bps).MarkTrue(BuildPlanConditionBuildSucceeded)
	case sc.Status == corev1.ConditionFalse:
		buildplanCondSet.Manage(bps).MarkFalse(BuildPlanConditionBuildSucceeded, sc.Reason, sc.Message)
	}
}
