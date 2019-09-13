/*
MIT License

Copyright (c) Microsoft Corporation. All rights reserved.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true

// ScaledObject is a spoecification for a ScaledObject resource
type ScaledObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScaledObjectSpec   `json:"spec"`
	Status ScaledObjectStatus `json:"status"`
}

// ScaledObjectSpec is the spec for a ScaledObject resource
type ScaledObjectSpec struct {
	ScaleTargetRef  ObjectReference `json:"scaleTargetRef"`
	PollingInterval *int32          `json:"pollingInterval"`
	CooldownPeriod  *int32          `json:"cooldownPeriod"`
	MinReplicaCount *int32          `json:"minReplicaCount"`
	MaxReplicaCount *int32          `json:"maxReplicaCount"`
	Triggers        []ScaleTriggers `json:"triggers"`
}

// ObjectReference holds the a reference to the deployment this
// ScaledObject applies
type ObjectReference struct {
	DeploymentName string `json:"deploymentName"`
	ContainerName  string `json:"containerName"`
}

type ScaleTriggers struct {
	Type     string            `json:"type"`
	Name     string            `json:"name"`
	Metadata map[string]string `json:"metadata"`
}

// ScaledObjectStatus is the status for a ScaledObject resource
type ScaledObjectStatus struct {
	LastActiveTime  *metav1.Time `json:"lastActiveTime,omitempty"`
	CurrentReplicas int32        `json:"currentReplicas"`
	DesiredReplicas int32        `json:"desiredReplicas"`
}

// +kubebuilder:object:root=true

// ScaledObjectList is a list of ScaledObject resources
type ScaledObjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ScaledObject `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScaledObject{}, &ScaledObjectList{})
}
