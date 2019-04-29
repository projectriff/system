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
	"github.com/knative/pkg/apis"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	"github.com/knative/pkg/kmeta"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RequestProcessor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RequestProcessorSpec   `json:"spec"`
	Status RequestProcessorStatus `json:"status"`
}

var (
	_ apis.Validatable   = (*RequestProcessor)(nil)
	_ apis.Defaultable   = (*RequestProcessor)(nil)
	_ kmeta.OwnerRefable = (*RequestProcessor)(nil)
)

type RequestProcessorSpec []RequestProcessorSpecItem

type RequestProcessorSpecItem struct {
	Tag            string `json:"tag,omitempty"`
	Percent        *int   `json:"percent,omitempty"`
	Build          *Build `json:"build,omitempty"`
	corev1.PodSpec `json:",inline"`
}

type Build struct {
	Application    *buildv1alpha1.ApplicationSpec `json:"application,omitempty"`
	ApplicationRef string                         `json:"applicationRef,omitempty"`
	Function       *buildv1alpha1.FunctionSpec    `json:"function,omitempty"`
	FunctionRef    string                         `json:"functionRef,omitempty"`
}

type RequestProcessorStatus struct {
	duckv1alpha1.Status `json:",inline"`

	Builds             []*corev1.ObjectReference `json:"build,omitempty"`
	ConfigurationNames []string                  `json:"configurationNames,omitempty"`
	RouteName          string                    `json:"routeName,omitempty"`

	Domain  string                    `json:"domain,omitempty"`
	Address *duckv1alpha1.Addressable `json:"address,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RequestProcessorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []RequestProcessor `json:"items"`
}

func (*RequestProcessor) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("RequestProcessor")
}
