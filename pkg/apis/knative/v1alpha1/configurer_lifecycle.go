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
	ConfigurerConditionReady                                   = knapis.ConditionReady
	ConfigurerConditionConfigurationReady knapis.ConditionType = "ConfigurationReady"
	ConfigurerConditionRouteReady         knapis.ConditionType = "RouteReady"
)

var configurerCondSet = knapis.NewLivingConditionSet(
	ConfigurerConditionConfigurationReady,
	ConfigurerConditionRouteReady,
)

func (cs *ConfigurerStatus) GetObservedGeneration() int64 {
	return cs.ObservedGeneration
}

func (cs *ConfigurerStatus) IsReady() bool {
	return configurerCondSet.Manage(cs).IsHappy()
}

func (*ConfigurerStatus) GetReadyConditionType() knapis.ConditionType {
	return ConfigurerConditionReady
}

func (cs *ConfigurerStatus) GetCondition(t knapis.ConditionType) *knapis.Condition {
	return configurerCondSet.Manage(cs).GetCondition(t)
}

func (cs *ConfigurerStatus) InitializeConditions() {
	configurerCondSet.Manage(cs).InitializeConditions()
}

func (cs *ConfigurerStatus) MarkConfigurationNotOwned(name string) {
	configurerCondSet.Manage(cs).MarkFalse(ConfigurerConditionConfigurationReady, "NotOwned",
		"There is an existing Configuration %q that we do not own.", name)
}

func (cs *ConfigurerStatus) PropagateConfigurationStatus(kcs *servingv1alpha1.ConfigurationStatus) {
	sc := kcs.GetCondition(servingv1alpha1.ConfigurationConditionReady)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		configurerCondSet.Manage(cs).MarkUnknown(ConfigurerConditionConfigurationReady, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		configurerCondSet.Manage(cs).MarkTrue(ConfigurerConditionConfigurationReady)
	case sc.Status == corev1.ConditionFalse:
		configurerCondSet.Manage(cs).MarkFalse(ConfigurerConditionConfigurationReady, sc.Reason, sc.Message)
	}
}

func (cs *ConfigurerStatus) MarkRouteNotOwned(name string) {
	configurerCondSet.Manage(cs).MarkFalse(ConfigurerConditionRouteReady, "NotOwned",
		"There is an existing Route %q that we do not own.", name)
}

func (cs *ConfigurerStatus) PropagateRouteStatus(rs *servingv1alpha1.RouteStatus) {
	cs.Address = rs.Address
	cs.URL = rs.URL

	sc := rs.GetCondition(servingv1alpha1.RouteConditionReady)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		configurerCondSet.Manage(cs).MarkUnknown(ConfigurerConditionRouteReady, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		configurerCondSet.Manage(cs).MarkTrue(ConfigurerConditionRouteReady)
	case sc.Status == corev1.ConditionFalse:
		configurerCondSet.Manage(cs).MarkFalse(ConfigurerConditionRouteReady, sc.Reason, sc.Message)
	}
}
