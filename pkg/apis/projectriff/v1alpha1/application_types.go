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
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec   `json:"spec"`
	Status ApplicationStatus `json:"status"`
}

type ApplicationSpec struct {
	Image string           `json:"image"`
	Build ApplicationBuild `json:"build"`
	Run   ApplicationRun   `json:"run"`
}

type ApplicationBuild struct {
	Annotations map[string]string  `json:"annotations,omitempty"`
	Template    string             `json:"template"`
	CacheSize   *resource.Quantity `json:"cacheSize,omitempty"`
	Arguments   []BuildArgument    `json:"arguments,omitempty"`
	Source      *Source            `json:"source,omitempty"`
}

type BuildArgument struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Source struct {
	Git     *GitSource `json:"git"`
	SubPath string     `json:"subPath,omitempty"`
}

type GitSource struct {
	Revision string `json:"revision"`
	URL      string `json:"url"`
}

type ApplicationRun struct {
	EnvFrom   []corev1.EnvFromSource      `json:"envFrom,omitempty"`
	Env       []corev1.EnvVar             `json:"env,omitempty" patchStrategy:"merge" patchMergeKey:"name"`
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

const (
	ApplicationConditionReady                                      = duckv1alpha1.ConditionReady
	ApplicationConditionServiceReady    duckv1alpha1.ConditionType = "ServiceReady"
	ApplicationConditionBuildCacheReady duckv1alpha1.ConditionType = "BuildCacheReady"
)

var applicationCondSet = duckv1alpha1.NewLivingConditionSet(ApplicationConditionServiceReady, ApplicationConditionBuildCacheReady)

type ApplicationStatus struct {
	Conditions         duckv1alpha1.Conditions   `json:"conditions,omitempty"`
	Address            *duckv1alpha1.Addressable `json:"address,omitempty"`
	Domain             string                    `json:"domain,omitempty"`
	BuildCacheName     string                    `json:"cacheVolumeName"`
	ObservedGeneration int64                     `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Application `json:"items"`
}

func (*Application) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Application")
}

func (as *ApplicationStatus) IsReady() bool {
	return applicationCondSet.Manage(as).IsHappy()
}

func (as *ApplicationStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return applicationCondSet.Manage(as).GetCondition(t)
}

func (as *ApplicationStatus) InitializeConditions() {
	applicationCondSet.Manage(as).InitializeConditions()
}

// TODO move into application reconciler
func (as *ApplicationStatus) MarkServiceNotOwned(name string) {
	applicationCondSet.Manage(as).MarkFalse(ApplicationConditionServiceReady, "NotOwned",
		fmt.Sprintf("There is an existing Service %q that we do not own.", name))
}

// TODO move into application reconciler
func (as *ApplicationStatus) MarkBuildCacheNotOwned(name string) {
	applicationCondSet.Manage(as).MarkFalse(ApplicationConditionBuildCacheReady, "NotOwned",
		fmt.Sprintf("There is an existing PersistentVolumeClaim %q that we do not own.", name))
}

// TODO move into application reconciler
func (as *ApplicationStatus) PropagateServiceStatus(ss *servingv1alpha1.ServiceStatus) {
	as.Address = ss.Address
	as.Domain = ss.Domain

	sc := ss.GetCondition(servingv1alpha1.ServiceConditionReady)
	if sc == nil {
		return
	}
	switch {
	case sc.Status == corev1.ConditionUnknown:
		applicationCondSet.Manage(as).MarkUnknown(ApplicationConditionServiceReady, sc.Reason, sc.Message)
	case sc.Status == corev1.ConditionTrue:
		applicationCondSet.Manage(as).MarkTrue(ApplicationConditionServiceReady)
	case sc.Status == corev1.ConditionFalse:
		applicationCondSet.Manage(as).MarkFalse(ApplicationConditionServiceReady, sc.Reason, sc.Message)
	}
}

// TODO move into application reconciler
func (as *ApplicationStatus) PropagateBuildCacheStatus(pvc *corev1.PersistentVolumeClaim) {
	if pvc == nil {
		as.BuildCacheName = ""
		applicationCondSet.Manage(as).MarkTrue(ApplicationConditionBuildCacheReady)
		return
	}

	as.BuildCacheName = pvc.Name

	switch pvc.Status.Phase {
	case corev1.ClaimPending:
		// used for PersistentVolumeClaims that are not yet bound
		applicationCondSet.Manage(as).MarkUnknown(ApplicationConditionBuildCacheReady, string(pvc.Status.Phase), "volume claim is not yet bound")
	case corev1.ClaimBound:
		// used for PersistentVolumeClaims that are bound
		applicationCondSet.Manage(as).MarkTrue(ApplicationConditionBuildCacheReady)
	case corev1.ClaimLost:
		// used for PersistentVolumeClaims that lost their underlying PersistentVolume
		applicationCondSet.Manage(as).MarkFalse(ApplicationConditionBuildCacheReady, string(pvc.Status.Phase), "volume claim lost its underlying volume")
	}
}
