/*
Copyright 2018 The Knative Authors

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
	"fmt"

	"github.com/knative/pkg/kmeta"
	"github.com/projectriff/system/pkg/apis/core"
	corev1alpha1 "github.com/projectriff/system/pkg/apis/core/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/corehandler/resources/names"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// MakeService creates a Service from a Handler object.
func MakeService(h *corev1alpha1.Handler) (*corev1.Service, error) {
	if h.Status.DeploymentName == "" {
		return nil, fmt.Errorf("unable to create Service, waiting for Deployment")
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Service(h),
			Namespace: h.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(h),
			},
			Labels: makeLabels(h),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
			},
			Selector: map[string]string{
				core.HandlerLabelKey: h.Name,
			},
		},
	}

	return service, nil
}
