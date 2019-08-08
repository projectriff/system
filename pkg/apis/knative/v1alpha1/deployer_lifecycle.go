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
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

const (
	DeployerConditionReady                                   = knapis.ConditionReady
	DeployerConditionConfigurationReady knapis.ConditionType = "ConfigurationReady"
	DeployerConditionRouteReady         knapis.ConditionType = "RouteReady"
)

var deployerCondSet = knapis.NewLivingConditionSet(
	DeployerConditionConfigurationReady,
	DeployerConditionRouteReady,
)

func (cs *DeployerStatus) GetObservedGeneration() int64 {
	return cs.ObservedGeneration
}

func (cs *DeployerStatus) IsReady() bool {
	return deployerCondSet.Manage(cs).IsHappy()
}

func (*DeployerStatus) GetReadyConditionType() knapis.ConditionType {
	return DeployerConditionReady
}

func (cs *DeployerStatus) GetCondition(t knapis.ConditionType) *knapis.Condition {
	return deployerCondSet.Manage(cs).GetCondition(t)
}

func (cs *DeployerStatus) InitializeConditions() {
	deployerCondSet.Manage(cs).InitializeConditions()
}

func (cs *DeployerStatus) MarkConfigurationNotOwned(name string) {
	deployerCondSet.Manage(cs).MarkFalse(DeployerConditionConfigurationReady, "NotOwned",
		"There is an existing Configuration %q that we do not own.", name)
}

func (cs *DeployerStatus) PropagateConfigurationStatus(kcs *servingv1alpha1.ConfigurationStatus) {
	sc := kcs.GetCondition(servingv1alpha1.ConfigurationConditionReady)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		deployerCondSet.Manage(cs).MarkUnknown(DeployerConditionConfigurationReady, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		deployerCondSet.Manage(cs).MarkTrue(DeployerConditionConfigurationReady)
	case sc.Status == corev1.ConditionFalse:
		deployerCondSet.Manage(cs).MarkFalse(DeployerConditionConfigurationReady, sc.Reason, sc.Message)
	}
}

func (cs *DeployerStatus) MarkRouteNotOwned(name string) {
	deployerCondSet.Manage(cs).MarkFalse(DeployerConditionRouteReady, "NotOwned",
		"There is an existing Route %q that we do not own.", name)
}

func (cs *DeployerStatus) PropagateRouteStatus(rs *servingv1alpha1.RouteStatus) {
	cs.Address = rs.Address
	cs.URL = rs.URL

	sc := rs.GetCondition(servingv1alpha1.RouteConditionReady)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		deployerCondSet.Manage(cs).MarkUnknown(DeployerConditionRouteReady, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		deployerCondSet.Manage(cs).MarkTrue(DeployerConditionRouteReady)
	case sc.Status == corev1.ConditionFalse:
		deployerCondSet.Manage(cs).MarkFalse(DeployerConditionRouteReady, sc.Reason, sc.Message)
	}
}
