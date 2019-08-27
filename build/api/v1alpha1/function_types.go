/*
Copyright 2019 The original author or authors.

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// FunctionSpec defines the desired state of Function
type FunctionSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Image repository to push built images. May contain a leading underscore
	// to have the default image prefix applied, or be `_` to combine the default
	// image prefix with the resource's name as a default value.
	Image string `json:"image"`

	// CacheSize of persistent volume to store resources between builds
	CacheSize *resource.Quantity `json:"cacheSize,omitempty"`

	// Source location. Required for on cluster builds.
	Source *Source `json:"source,omitempty"`

	// Artifact file containing the function within the build workspace.
	Artifact string `json:"artifact,omitempty"`

	// Handler name of the method or class to invoke. The value depends on the
	// invoker.
	Handler string `json:"handler,omitempty"`

	// Invoker language runtime name. Detected by default.
	Invoker string `json:"invoker,omitempty"`
}

type Source struct {
	// Git source location to clone and checkout for the build.
	Git *GitSource `json:"git"`

	// SubPath within the source to mount. Files outside of the sub path will
	// not be available to tbe build.
	SubPath string `json:"subPath,omitempty"`
}

type GitSource struct {
	// Revision in the git repository to checkout. May be any valid refspec
	// including commit sha, tags or branches.
	Revision string `json:"revision"`

	// URL to a cloneable git repository.
	URL string `json:"url"`
}

// FunctionStatus defines the observed state of Function
type FunctionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	BuildStatus `json:",inline"`
}

type BuildStatus struct {
	// BuildCacheName is the name of the PersistentVolumeClaim used as a cache
	// for intermediate build resources.
	BuildCacheName string `json:"buildCacheName,omitempty"`

	// BuildName is the name of the Knative Build backing this build.
	BuildName string `json:"buildName,omitempty"`

	// LatestImage is the most recent image for this build.
	LatestImage string `json:"latestImage,omitempty"`

	// TargetImage is the resolved image repository where built images are
	// pushed.
	TargetImage string `json:"targetImage,omitempty"`
}

// +kubebuilder:object:root=true

// Function is the Schema for the functions API
type Function struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FunctionSpec   `json:"spec,omitempty"`
	Status FunctionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FunctionList contains a list of Function
type FunctionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Function `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Function{}, &FunctionList{})
}
