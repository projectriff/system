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
 *
 */

package resources

import (
	"strings"

	"github.com/knative/pkg/kmeta"
	streamv1alpha1 "github.com/projectriff/system/pkg/apis/stream/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/processor/resources/names"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func MakeDeployment(proc *streamv1alpha1.Processor) (*appsv1.Deployment, error) {
	one := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            names.Deployment(proc),
			Namespace:       proc.Namespace,
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(proc)},
			Labels:          makeLabels(proc),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": proc.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": proc.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						corev1.Container{
							Name:  "processor",
							Image: "ericbottard/processor:grpc-be83df8",
							Env: []corev1.EnvVar{
								{
									Name:  "INPUTS",
									Value: strings.Join(proc.Status.InputAddresses, ","),
								},
								{
									Name:  "OUTPUTS",
									Value: strings.Join(proc.Status.OutputAddresses, ","),
								},
								{
									Name:  "GROUP",
									Value: proc.Name,
								},
								{
									Name:  "FUNCTION",
									Value: "localhost:8080",
								},
							},
						},
						corev1.Container{
							Name:  "function",
							Image: proc.Status.FunctionImage,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
								},
							},
						},
					},
				},
			},
		},
	}

	return deployment, nil
}
