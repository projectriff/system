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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Stream struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StreamSpec   `json:"spec"`
	Status StreamStatus `json:"status"`
}

type StreamSpec struct {
	Provider string `json:"provider"`
}

const (
	StreamConditionReady                                        = duckv1alpha1.ConditionReady
	StreamConditionResourceAvailable duckv1alpha1.ConditionType = "ResourceAvailable"
)

var streamCondSet = duckv1alpha1.NewLivingConditionSet(StreamConditionResourceAvailable)

type StreamStatus struct {
	Address            StreamAddress           `json:"address"`
	Conditions         duckv1alpha1.Conditions `json:"conditions,omitempty"`
	ObservedGeneration int64                   `json:"observedGeneration,omitempty"`
}

type StreamAddress struct {
	Gateway string `json:"gateway,omitempty"`
	Topic   string `json:"topic,omitempty"`
}

func (a StreamAddress) String() string {
	return fmt.Sprintf("%s/%s", a.Gateway, a.Topic)
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type StreamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Stream `json:"items"`
}

func (*Stream) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Stream")
}

func (status *StreamStatus) IsReady() bool {
	return streamCondSet.Manage(status).IsHappy()
}

func (status *StreamStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return streamCondSet.Manage(status).GetCondition(t)
}

func (status *StreamStatus) InitializeConditions() {
	streamCondSet.Manage(status).InitializeConditions()
}
