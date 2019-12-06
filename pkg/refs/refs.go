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

package refs

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TypedLocalObjectReference(name string, gk schema.GroupKind) *corev1.TypedLocalObjectReference {
	if name == "" || gk.Empty() {
		return nil
	}

	ref := &corev1.TypedLocalObjectReference{
		Kind: gk.Kind,
		Name: name,
	}
	if gk.Group != "" && gk.Group != "core" {
		ref.APIGroup = &gk.Group
	}
	return ref
}

func TypedLocalObjectReferenceForObject(obj runtime.Object, scheme *runtime.Scheme) *corev1.TypedLocalObjectReference {
	if obj == nil || scheme == nil {
		return nil
	}
	meta, ok := obj.(metav1.Object)
	if !ok {
		return nil
	}

	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil || len(gvks) == 0 {
		panic(fmt.Errorf("Unregistered runtime object: %v", err))
	}
	return TypedLocalObjectReference(meta.GetName(), gvks[0].GroupKind())
}
