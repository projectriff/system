/*
Copyright 2019 the original author or authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/projectriff/system/pkg/apis"
)

const (
	InMemoryProviderConditionReady                                         = apis.ConditionReady
	InMemoryProviderConditionGatewayDeploymentReady     apis.ConditionType = "GatewayDeploymentReady"
	InMemoryProviderConditionGatewayServiceReady        apis.ConditionType = "GatewayServiceReady"
	InMemoryProviderConditionProvisionerDeploymentReady apis.ConditionType = "ProvisionerDeploymentReady"
	InMemoryProviderConditionProvisionerServiceReady    apis.ConditionType = "ProvisionerServiceReady"
)

var inMemoryProviderCondSet = apis.NewLivingConditionSet(
	InMemoryProviderConditionGatewayDeploymentReady,
	InMemoryProviderConditionGatewayServiceReady,
	InMemoryProviderConditionProvisionerDeploymentReady,
	InMemoryProviderConditionProvisionerServiceReady,
)

func (ps *InMemoryProviderStatus) GetObservedGeneration() int64 {
	return ps.ObservedGeneration
}

func (ps *InMemoryProviderStatus) IsReady() bool {
	return inMemoryProviderCondSet.Manage(ps).IsHappy()
}

func (*InMemoryProviderStatus) GetReadyConditionType() apis.ConditionType {
	return InMemoryProviderConditionReady
}

func (ps *InMemoryProviderStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return inMemoryProviderCondSet.Manage(ps).GetCondition(t)
}

func (ps *InMemoryProviderStatus) InitializeConditions() {
	inMemoryProviderCondSet.Manage(ps).InitializeConditions()
}

func (ps *InMemoryProviderStatus) PropagateGatewayDeploymentStatus(cds *appsv1.DeploymentStatus) {
	var available, progressing *appsv1.DeploymentCondition
	for i := range cds.Conditions {
		switch cds.Conditions[i].Type {
		case appsv1.DeploymentAvailable:
			available = &cds.Conditions[i]
		case appsv1.DeploymentProgressing:
			progressing = &cds.Conditions[i]
		}
	}
	if available == nil || progressing == nil {
		return
	}
	if progressing.Status == corev1.ConditionTrue && available.Status == corev1.ConditionFalse {
		// DeploymentAvailable is False while progressing, avoid reporting InMemoryProviderConditionReady as False
		inMemoryProviderCondSet.Manage(ps).MarkUnknown(InMemoryProviderConditionGatewayDeploymentReady, progressing.Reason, progressing.Message)
		return
	}
	switch {
	case available.Status == corev1.ConditionUnknown:
		inMemoryProviderCondSet.Manage(ps).MarkUnknown(InMemoryProviderConditionGatewayDeploymentReady, available.Reason, available.Message)
	case available.Status == corev1.ConditionTrue:
		inMemoryProviderCondSet.Manage(ps).MarkTrue(InMemoryProviderConditionGatewayDeploymentReady)
	case available.Status == corev1.ConditionFalse:
		inMemoryProviderCondSet.Manage(ps).MarkFalse(InMemoryProviderConditionGatewayDeploymentReady, available.Reason, available.Message)
	}
}

func (ps *InMemoryProviderStatus) PropagateGatewayServiceStatus(ss *corev1.ServiceStatus) {
	// services don't have meaningful status
	inMemoryProviderCondSet.Manage(ps).MarkTrue(InMemoryProviderConditionGatewayServiceReady)
}

func (ps *InMemoryProviderStatus) PropagateProvisionerDeploymentStatus(cds *appsv1.DeploymentStatus) {
	var available, progressing *appsv1.DeploymentCondition
	for i := range cds.Conditions {
		switch cds.Conditions[i].Type {
		case appsv1.DeploymentAvailable:
			available = &cds.Conditions[i]
		case appsv1.DeploymentProgressing:
			progressing = &cds.Conditions[i]
		}
	}
	if available == nil || progressing == nil {
		return
	}
	if progressing.Status == corev1.ConditionTrue && available.Status == corev1.ConditionFalse {
		// DeploymentAvailable is False while progressing, avoid reporting InMemoryProviderConditionReady as False
		inMemoryProviderCondSet.Manage(ps).MarkUnknown(InMemoryProviderConditionProvisionerDeploymentReady, progressing.Reason, progressing.Message)
		return
	}
	switch {
	case available.Status == corev1.ConditionUnknown:
		inMemoryProviderCondSet.Manage(ps).MarkUnknown(InMemoryProviderConditionProvisionerDeploymentReady, available.Reason, available.Message)
	case available.Status == corev1.ConditionTrue:
		inMemoryProviderCondSet.Manage(ps).MarkTrue(InMemoryProviderConditionProvisionerDeploymentReady)
	case available.Status == corev1.ConditionFalse:
		inMemoryProviderCondSet.Manage(ps).MarkFalse(InMemoryProviderConditionProvisionerDeploymentReady, available.Reason, available.Message)
	}
}

func (ps *InMemoryProviderStatus) PropagateProvisionerServiceStatus(ss *corev1.ServiceStatus) {
	// services don't have meaningful status
	inMemoryProviderCondSet.Manage(ps).MarkTrue(InMemoryProviderConditionProvisionerServiceReady)
}
