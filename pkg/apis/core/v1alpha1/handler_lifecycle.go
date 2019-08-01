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
	knapis "github.com/knative/pkg/apis"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	HandlerConditionReady                                = knapis.ConditionReady
	HandlerConditionDeploymentReady knapis.ConditionType = "DeploymentReady"
	HandlerConditionServiceReady    knapis.ConditionType = "ServiceReady"
)

var handlerCondSet = knapis.NewLivingConditionSet(
	HandlerConditionDeploymentReady,
	HandlerConditionServiceReady,
)

func (hs *HandlerStatus) GetObservedGeneration() int64 {
	return hs.ObservedGeneration
}

func (hs *HandlerStatus) IsReady() bool {
	return handlerCondSet.Manage(hs).IsHappy()
}

func (*HandlerStatus) GetReadyConditionType() knapis.ConditionType {
	return HandlerConditionReady
}

func (hs *HandlerStatus) GetCondition(t knapis.ConditionType) *knapis.Condition {
	return handlerCondSet.Manage(hs).GetCondition(t)
}

func (hs *HandlerStatus) InitializeConditions() {
	handlerCondSet.Manage(hs).InitializeConditions()
}

func (hs *HandlerStatus) MarkDeploymentNotOwned(name string) {
	handlerCondSet.Manage(hs).MarkFalse(HandlerConditionDeploymentReady, "NotOwned",
		"There is an existing Deployment %q that we do not own.", name)
}

func (hs *HandlerStatus) PropagateDeploymentStatus(ds *appsv1.DeploymentStatus) {
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
		handlerCondSet.Manage(hs).MarkUnknown(HandlerConditionDeploymentReady, ac.Reason, ac.Message)
	case ac.Status == corev1.ConditionTrue:
		handlerCondSet.Manage(hs).MarkTrue(HandlerConditionDeploymentReady)
	case ac.Status == corev1.ConditionFalse:
		handlerCondSet.Manage(hs).MarkFalse(HandlerConditionDeploymentReady, ac.Reason, ac.Message)
	}
}

func (hs *HandlerStatus) MarkServiceNotOwned(name string) {
	handlerCondSet.Manage(hs).MarkFalse(HandlerConditionServiceReady, "NotOwned",
		"There is an existing Service %q that we do not own.", name)
}

func (hs *HandlerStatus) PropagateServiceStatus(ss *corev1.ServiceStatus) {
	// services don't have meaningful status
	handlerCondSet.Manage(hs).MarkTrue(HandlerConditionServiceReady)
}
