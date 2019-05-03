/*
Copyright 2019 The Knative Authors

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

package reconciler

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type accessor interface {
	GroupVersionKind() schema.GroupVersionKind
	GetNamespace() string
	GetName() string
}

func MakeObjectRef(a accessor, gvk schema.GroupVersionKind) corev1.ObjectReference {
	// We can't always rely on the TypeMeta being populated.
	// See: https://github.com/knative/serving/issues/2372
	// Also: https://github.com/kubernetes/apiextensions-apiserver/issues/29
	// gvk := a.GroupVersionKind()
	apiVersion, kind := gvk.ToAPIVersionAndKind()
	return corev1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       kind,
		Namespace:  a.GetNamespace(),
		Name:       a.GetName(),
	}
}
