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
package resources

import (
	"encoding/json"
	"github.com/projectriff/system/pkg/apis/streaming/v1alpha1/resources/names"
	"strings"

	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func MakeDeployment(proc *streamingv1alpha1.Processor) (*appsv1.Deployment, error) {
	one := int32(1)
	environmentVariables, err := computeEnvironmentVariables(proc)
	if err != nil {
		return nil, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            names.Deployment(proc),
			Namespace:       proc.Namespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(proc.GetObjectMeta(), proc.GroupVersionKind())},
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
						{
							Name:            "processor",
							Image:           "ericbottard/processor:grpc",
							ImagePullPolicy: corev1.PullAlways,
							Env:             environmentVariables,
						},
						{
							Name:  "function",
							Image: proc.Status.FunctionImage,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8081,
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

func computeEnvironmentVariables(processor *streamingv1alpha1.Processor) ([]corev1.EnvVar, error) {
	contentTypesJson, err := serializeContentTypes(processor.Status.OutputContentTypes)
	if err != nil {
		return nil, err
	}
	return []corev1.EnvVar{
		{
			Name:  "INPUTS",
			Value: strings.Join(processor.Status.InputAddresses, ","),
		},
		{
			Name:  "OUTPUTS",
			Value: strings.Join(processor.Status.OutputAddresses, ","),
		},
		{
			Name:  "GROUP",
			Value: processor.Name,
		},
		{
			Name:  "FUNCTION",
			Value: "localhost:8081",
		},
		{
			Name:  "OUTPUT_CONTENT_TYPES",
			Value: contentTypesJson,
		},
	}, nil
}

type outputContentType struct {
	OutputIndex int    `json:"outputIndex"`
	ContentType string `json:"contentType"`
}

func serializeContentTypes(outputContentTypes []string) (string, error) {
	outputCount := len(outputContentTypes)
	result := make([]outputContentType, outputCount)
	for i := 0; i < outputCount; i++ {
		result[i] = outputContentType{
			OutputIndex: i,
			ContentType: outputContentTypes[i],
		}
	}
	bytes, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
