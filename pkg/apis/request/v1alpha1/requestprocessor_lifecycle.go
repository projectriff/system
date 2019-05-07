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
	RequestProcessorConditionReady                                          = duckv1alpha1.ConditionReady
	RequestProcessorConditionBuildsReady         duckv1alpha1.ConditionType = "BuildsReady"
	RequestProcessorConditionConfigurationsReady duckv1alpha1.ConditionType = "ConfigurationsReady"
	RequestProcessorConditionRouteReady          duckv1alpha1.ConditionType = "RouteReady"
)

var requestprocessorCondSet = duckv1alpha1.NewLivingConditionSet(
	RequestProcessorConditionBuildsReady,
	RequestProcessorConditionConfigurationsReady,
	RequestProcessorConditionRouteReady,
)

func (rps *RequestProcessorStatus) IsReady() bool {
	return requestprocessorCondSet.Manage(rps).IsHappy()
}

func (rps *RequestProcessorStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return requestprocessorCondSet.Manage(rps).GetCondition(t)
}

func (rps *RequestProcessorStatus) InitializeConditions() {
	requestprocessorCondSet.Manage(rps).InitializeConditions()
}

func (rps *RequestProcessorStatus) MarkBuildsCreated() {
	requestprocessorCondSet.Manage(rps).MarkTrue(RequestProcessorConditionBuildsReady)
}

func (rps *RequestProcessorStatus) MarkBuildsFailed(reason, messageFormat string, messageA ...interface{}) {
	requestprocessorCondSet.Manage(rps).MarkFalse(RequestProcessorConditionBuildsReady, reason, messageFormat, messageA...)
}

func (rps *RequestProcessorStatus) MarkBuildsUnknown(reason, messageFormat string, messageA ...interface{}) {
	requestprocessorCondSet.Manage(rps).MarkUnknown(RequestProcessorConditionBuildsReady, reason, messageFormat, messageA...)
}

func (rps *RequestProcessorStatus) MarkBuildNotOwned(kind, name string) {
	requestprocessorCondSet.Manage(rps).MarkFalse(RequestProcessorConditionBuildsReady, "NotOwned",
		"There is an existing %s %q that we do not own.", kind, name)
}

func (rps *RequestProcessorStatus) MarkConfigurationsReady() {
	requestprocessorCondSet.Manage(rps).MarkTrue(RequestProcessorConditionConfigurationsReady)
}

func (rps *RequestProcessorStatus) MarkConfigurationsFailed(reason, messageFormat string, messageA ...interface{}) {
	requestprocessorCondSet.Manage(rps).MarkFalse(RequestProcessorConditionConfigurationsReady, reason, messageFormat, messageA...)
}

func (rps *RequestProcessorStatus) MarkConfigurationsUnknown(reason, messageFormat string, messageA ...interface{}) {
	requestprocessorCondSet.Manage(rps).MarkUnknown(RequestProcessorConditionConfigurationsReady, reason, messageFormat, messageA...)
}

func (rps *RequestProcessorStatus) MarkConfigurationNotOwned(name string) {
	requestprocessorCondSet.Manage(rps).MarkFalse(RequestProcessorConditionConfigurationsReady, "NotOwned",
		"There is an existing Configuration %q that we do not own.", name)
}

func (rps *RequestProcessorStatus) MarkRouteNotOwned(name string) {
	requestprocessorCondSet.Manage(rps).MarkFalse(RequestProcessorConditionRouteReady, "NotOwned",
		"There is an existing Route %q that we do not own.", name)
}

func (rps *RequestProcessorStatus) PropagateRouteStatus(rs *servingv1alpha1.RouteStatus) {
	rps.Address = rs.Address
	rps.Domain = rs.Domain

	sc := rs.GetCondition(servingv1alpha1.RouteConditionReady)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		requestprocessorCondSet.Manage(rps).MarkUnknown(RequestProcessorConditionRouteReady, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		requestprocessorCondSet.Manage(rps).MarkTrue(RequestProcessorConditionRouteReady)
	case sc.Status == corev1.ConditionFalse:
		requestprocessorCondSet.Manage(rps).MarkFalse(RequestProcessorConditionRouteReady, sc.Reason, sc.Message)
	}
}
