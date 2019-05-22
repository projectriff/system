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
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

const (
	RouteConditionReady                                           = duckv1alpha1.ConditionReady
	RouteConditionKnConfigurationReady duckv1alpha1.ConditionType = "KnConfigurationReady"
	RouteConditionKnRouteReady         duckv1alpha1.ConditionType = "KnRouteReady"
)

var routeCondSet = duckv1alpha1.NewLivingConditionSet(
	RouteConditionKnConfigurationReady,
	RouteConditionKnRouteReady,
)

func (rs *RouteStatus) IsReady() bool {
	return routeCondSet.Manage(rs).IsHappy()
}

func (rs *RouteStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return routeCondSet.Manage(rs).GetCondition(t)
}

func (rs *RouteStatus) InitializeConditions() {
	routeCondSet.Manage(rs).InitializeConditions()
}

func (rs *RouteStatus) MarkKnConfigurationNotOwned(name string) {
	routeCondSet.Manage(rs).MarkFalse(RouteConditionKnConfigurationReady, "NotOwned",
		"There is an existing Configuration %q that we do not own.", name)
}

func (rs *RouteStatus) PropagateKnConfigurationStatus(cs *servingv1alpha1.ConfigurationStatus) {
	sc := cs.GetCondition(servingv1alpha1.ConfigurationConditionReady)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		routeCondSet.Manage(rs).MarkUnknown(RouteConditionKnConfigurationReady, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		routeCondSet.Manage(rs).MarkTrue(RouteConditionKnConfigurationReady)
	case sc.Status == corev1.ConditionFalse:
		routeCondSet.Manage(rs).MarkFalse(RouteConditionKnConfigurationReady, sc.Reason, sc.Message)
	}
}

func (rs *RouteStatus) MarkKnRouteNotOwned(name string) {
	routeCondSet.Manage(rs).MarkFalse(RouteConditionKnRouteReady, "NotOwned",
		"There is an existing Route %q that we do not own.", name)
}

func (rs *RouteStatus) PropagateKnRouteStatus(knrs *servingv1alpha1.RouteStatus) {
	rs.Address = knrs.Address
	rs.Domain = knrs.Domain

	sc := knrs.GetCondition(servingv1alpha1.RouteConditionReady)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		routeCondSet.Manage(rs).MarkUnknown(RouteConditionKnRouteReady, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		routeCondSet.Manage(rs).MarkTrue(RouteConditionKnRouteReady)
	case sc.Status == corev1.ConditionFalse:
		routeCondSet.Manage(rs).MarkFalse(RouteConditionKnRouteReady, sc.Reason, sc.Message)
	}
}
