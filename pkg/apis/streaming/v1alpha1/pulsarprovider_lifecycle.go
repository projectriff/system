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
	PulsarProviderConditionReady                                         = apis.ConditionReady
	PulsarProviderConditionGatewayDeploymentReady     apis.ConditionType = "GatewayDeploymentReady"
	PulsarProviderConditionGatewayServiceReady        apis.ConditionType = "GatewayServiceReady"
	PulsarProviderConditionProvisionerDeploymentReady apis.ConditionType = "ProvisionerDeploymentReady"
	PulsarProviderConditionProvisionerServiceReady    apis.ConditionType = "ProvisionerServiceReady"
)

var pulsarProviderCondSet = apis.NewLivingConditionSet(
	PulsarProviderConditionGatewayDeploymentReady,
	PulsarProviderConditionGatewayServiceReady,
	PulsarProviderConditionProvisionerDeploymentReady,
	PulsarProviderConditionProvisionerServiceReady,
)

func (ps *PulsarProviderStatus) GetObservedGeneration() int64 {
	return ps.ObservedGeneration
}

func (ps *PulsarProviderStatus) IsReady() bool {
	return pulsarProviderCondSet.Manage(ps).IsHappy()
}

func (*PulsarProviderStatus) GetReadyConditionType() apis.ConditionType {
	return PulsarProviderConditionReady
}

func (ps *PulsarProviderStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return pulsarProviderCondSet.Manage(ps).GetCondition(t)
}

func (ps *PulsarProviderStatus) InitializeConditions() {
	pulsarProviderCondSet.Manage(ps).InitializeConditions()
}

func (ps *PulsarProviderStatus) PropagateGatewayDeploymentStatus(cds *appsv1.DeploymentStatus) {
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
		// DeploymentAvailable is False while progressing, avoid reporting PulsarProviderConditionReady as False
		providerCondSet.Manage(ps).MarkUnknown(PulsarProviderConditionGatewayDeploymentReady, progressing.Reason, progressing.Message)
		return
	}
	switch {
	case available.Status == corev1.ConditionUnknown:
		providerCondSet.Manage(ps).MarkUnknown(PulsarProviderConditionGatewayDeploymentReady, available.Reason, available.Message)
	case available.Status == corev1.ConditionTrue:
		providerCondSet.Manage(ps).MarkTrue(PulsarProviderConditionGatewayDeploymentReady)
	case available.Status == corev1.ConditionFalse:
		providerCondSet.Manage(ps).MarkFalse(PulsarProviderConditionGatewayDeploymentReady, available.Reason, available.Message)
	}
}

func (ps *PulsarProviderStatus) PropagateGatewayServiceStatus(ss *corev1.ServiceStatus) {
	// services don't have meaningful status
	providerCondSet.Manage(ps).MarkTrue(PulsarProviderConditionGatewayServiceReady)
}

func (ps *PulsarProviderStatus) PropagateProvisionerDeploymentStatus(cds *appsv1.DeploymentStatus) {
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
		// DeploymentAvailable is False while progressing, avoid reporting PulsarProviderConditionReady as False
		providerCondSet.Manage(ps).MarkUnknown(PulsarProviderConditionProvisionerDeploymentReady, progressing.Reason, progressing.Message)
		return
	}
	switch {
	case available.Status == corev1.ConditionUnknown:
		providerCondSet.Manage(ps).MarkUnknown(PulsarProviderConditionProvisionerDeploymentReady, available.Reason, available.Message)
	case available.Status == corev1.ConditionTrue:
		providerCondSet.Manage(ps).MarkTrue(PulsarProviderConditionProvisionerDeploymentReady)
	case available.Status == corev1.ConditionFalse:
		providerCondSet.Manage(ps).MarkFalse(PulsarProviderConditionProvisionerDeploymentReady, available.Reason, available.Message)
	}
}

func (ps *PulsarProviderStatus) PropagateProvisionerServiceStatus(ss *corev1.ServiceStatus) {
	// services don't have meaningful status
	providerCondSet.Manage(ps).MarkTrue(PulsarProviderConditionProvisionerServiceReady)
}
