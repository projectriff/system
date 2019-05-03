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
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	StreamProcessorConditionReady = duckv1alpha1.ConditionReady
	// TODO add aggregated streams ready status
	StreamProcessorConditionFunctionReady   duckv1alpha1.ConditionType = "FunctionReady"
	StreamProcessorConditionDeploymentReady duckv1alpha1.ConditionType = "DeploymentReady"
)

var streamprocessorCondSet = duckv1alpha1.NewLivingConditionSet(
	StreamProcessorConditionFunctionReady,
	StreamProcessorConditionDeploymentReady,
)

func (sps *StreamProcessorStatus) IsReady() bool {
	return streamprocessorCondSet.Manage(sps).IsHappy()
}

func (sps *StreamProcessorStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return streamprocessorCondSet.Manage(sps).GetCondition(t)
}

func (sps *StreamProcessorStatus) InitializeConditions() {
	streamprocessorCondSet.Manage(sps).InitializeConditions()
}

func (sps *StreamProcessorStatus) MarkDeploymentNotOwned(name string) {
	streamprocessorCondSet.Manage(sps).MarkFalse(StreamProcessorConditionDeploymentReady, "NotOwned",
		"There is an existing Deployment %q that we do not own.", name)
}

func (sps *StreamProcessorStatus) PropagateDeploymentStatus(ds *appsv1.DeploymentStatus) {
	var ac *appsv1.DeploymentCondition
	for _, c := range ds.Conditions {
		if c.Type == appsv1.DeploymentAvailable {
			ac = &c
			break
		}
	}
	if ac == nil {
		return
	}
	switch {
	case ac.Status == corev1.ConditionUnknown:
		streamprocessorCondSet.Manage(sps).MarkUnknown(StreamProcessorConditionDeploymentReady, ac.Reason, ac.Message)
	case ac.Status == corev1.ConditionTrue:
		streamprocessorCondSet.Manage(sps).MarkTrue(StreamProcessorConditionDeploymentReady)
	case ac.Status == corev1.ConditionFalse:
		streamprocessorCondSet.Manage(sps).MarkFalse(StreamProcessorConditionDeploymentReady, ac.Reason, ac.Message)
	}
}
