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

package resources

import (
	"github.com/projectriff/system/api/v1alpha1"
	"github.com/projectriff/system/controllers/names"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeBuildCache creates a PersistentVolumeClaim from an Function object.
func MakeBuildCache(f *v1alpha1.Function) (*corev1.PersistentVolumeClaim, error) {
	if f.Spec.CacheSize == nil {
		// no cache was requested
		return nil, nil
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.BuildCache(f),
			Namespace: f.Namespace,
			Labels:    makeLabels(f),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: *f.Spec.CacheSize,
				},
			},
		},
	}

	return pvc, nil
}
