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
	"fmt"

	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Function struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FunctionSpec   `json:"spec"`
	Status FunctionStatus `json:"status"`
}

type FunctionSpec struct {
	Image string        `json:"image"`
	Build FunctionBuild `json:"build"`
	Run   FunctionRun   `json:"run"`
}

type FunctionBuild struct {
	Artifact string  `json:"artifact,omitempty"`
	Handler  string  `json:"handler,omitempty"`
	Invoker  string  `json:"invoker,omitempty"`
	Source   *Source `json:"source,omitempty"`
}

type FunctionRun ApplicationRun

const (
	FunctionConditionReady                                       = duckv1alpha1.ConditionReady
	FunctionConditionApplicationReady duckv1alpha1.ConditionType = "ApplicationReady"
)

var functionCondSet = duckv1alpha1.NewLivingConditionSet(FunctionConditionApplicationReady)

type FunctionStatus struct {
	Conditions         duckv1alpha1.Conditions   `json:"conditions,omitempty"`
	Address            *duckv1alpha1.Addressable `json:"address,omitempty"`
	ObservedGeneration int64                     `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type FunctionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Function `json:"items"`
}

func (*Function) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Function")
}

func (fs *FunctionStatus) IsReady() bool {
	return functionCondSet.Manage(fs).IsHappy()
}

func (fs *FunctionStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return functionCondSet.Manage(fs).GetCondition(t)
}

func (fs *FunctionStatus) InitializeConditions() {
	functionCondSet.Manage(fs).InitializeConditions()
}

// TODO move into function reconciler
func (fs *FunctionStatus) MarkApplicationNotOwned(name string) {
	functionCondSet.Manage(fs).MarkFalse(FunctionConditionApplicationReady, "NotOwned",
		fmt.Sprintf("There is an existing Application %q that we do not own.", name))
}

// TODO move into function reconciler
func (fs *FunctionStatus) PropagateApplicationStatus(as *ApplicationStatus) {
	fs.Address = as.Address

	ac := as.GetCondition(ApplicationConditionReady)
	if ac == nil {
		return
	}
	switch {
	case ac.Status == corev1.ConditionUnknown:
		functionCondSet.Manage(fs).MarkUnknown(FunctionConditionApplicationReady, ac.Reason, ac.Message)
	case ac.Status == corev1.ConditionTrue:
		functionCondSet.Manage(fs).MarkTrue(FunctionConditionApplicationReady)
	case ac.Status == corev1.ConditionFalse:
		functionCondSet.Manage(fs).MarkFalse(FunctionConditionApplicationReady, ac.Reason, ac.Message)
	}
}
