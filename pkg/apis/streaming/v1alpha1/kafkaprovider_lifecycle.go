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
	KafkaProviderConditionReady                                         = apis.ConditionReady
	KafkaProviderConditionGatewayDeploymentReady     apis.ConditionType = "GatewayDeploymentReady"
	KafkaProviderConditionGatewayServiceReady        apis.ConditionType = "GatewayServiceReady"
	KafkaProviderConditionProvisionerDeploymentReady apis.ConditionType = "ProvisionerDeploymentReady"
	KafkaProviderConditionProvisionerServiceReady    apis.ConditionType = "ProvisionerServiceReady"
)

var providerCondSet = apis.NewLivingConditionSet(
	KafkaProviderConditionGatewayDeploymentReady,
	KafkaProviderConditionGatewayServiceReady,
	KafkaProviderConditionProvisionerDeploymentReady,
	KafkaProviderConditionProvisionerServiceReady,
)

func (ps *KafkaProviderStatus) GetObservedGeneration() int64 {
	return ps.ObservedGeneration
}

func (ps *KafkaProviderStatus) IsReady() bool {
	return providerCondSet.Manage(ps).IsHappy()
}

func (*KafkaProviderStatus) GetReadyConditionType() apis.ConditionType {
	return KafkaProviderConditionReady
}

func (ps *KafkaProviderStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return providerCondSet.Manage(ps).GetCondition(t)
}

func (ps *KafkaProviderStatus) InitializeConditions() {
	providerCondSet.Manage(ps).InitializeConditions()
}

func (ps *KafkaProviderStatus) PropagateGatewayDeploymentStatus(cds *appsv1.DeploymentStatus) {
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
		// DeploymentAvailable is False while progressing, avoid reporting KafkaProviderConditionReady as False
		providerCondSet.Manage(ps).MarkUnknown(KafkaProviderConditionGatewayDeploymentReady, progressing.Reason, progressing.Message)
		return
	}
	switch {
	case available.Status == corev1.ConditionUnknown:
		providerCondSet.Manage(ps).MarkUnknown(KafkaProviderConditionGatewayDeploymentReady, available.Reason, available.Message)
	case available.Status == corev1.ConditionTrue:
		providerCondSet.Manage(ps).MarkTrue(KafkaProviderConditionGatewayDeploymentReady)
	case available.Status == corev1.ConditionFalse:
		providerCondSet.Manage(ps).MarkFalse(KafkaProviderConditionGatewayDeploymentReady, available.Reason, available.Message)
	}
}

func (ps *KafkaProviderStatus) PropagateGatewayServiceStatus(ss *corev1.ServiceStatus) {
	// services don't have meaningful status
	providerCondSet.Manage(ps).MarkTrue(KafkaProviderConditionGatewayServiceReady)
}

func (ps *KafkaProviderStatus) PropagateProvisionerDeploymentStatus(cds *appsv1.DeploymentStatus) {
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
		// DeploymentAvailable is False while progressing, avoid reporting KafkaProviderConditionReady as False
		providerCondSet.Manage(ps).MarkUnknown(KafkaProviderConditionProvisionerDeploymentReady, progressing.Reason, progressing.Message)
		return
	}
	switch {
	case available.Status == corev1.ConditionUnknown:
		providerCondSet.Manage(ps).MarkUnknown(KafkaProviderConditionProvisionerDeploymentReady, available.Reason, available.Message)
	case available.Status == corev1.ConditionTrue:
		providerCondSet.Manage(ps).MarkTrue(KafkaProviderConditionProvisionerDeploymentReady)
	case available.Status == corev1.ConditionFalse:
		providerCondSet.Manage(ps).MarkFalse(KafkaProviderConditionProvisionerDeploymentReady, available.Reason, available.Message)
	}
}

func (ps *KafkaProviderStatus) PropagateProvisionerServiceStatus(ss *corev1.ServiceStatus) {
	// services don't have meaningful status
	providerCondSet.Manage(ps).MarkTrue(KafkaProviderConditionProvisionerServiceReady)
}
