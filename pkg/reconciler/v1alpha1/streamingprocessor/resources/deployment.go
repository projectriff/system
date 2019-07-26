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
	"encoding/json"
	"github.com/kedacore/keda/pkg/apis/keda/v1alpha1"
	"strings"

	"github.com/knative/pkg/kmeta"
	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/streamingprocessor/resources/names"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func MakeDeployment(proc *streamingv1alpha1.Processor) (*appsv1.Deployment, error) {
	zero := int32(0)
	environmentVariables, err := computeEnvironmentVariables(proc)
	if err != nil {
		return nil, err
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            names.Deployment(proc),
			Namespace:       proc.Namespace,
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(proc)},
			Labels:          makeLabels(proc),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &zero,
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

func MakeScaledObject(proc *streamingv1alpha1.Processor, deployment *appsv1.Deployment) (*v1alpha1.ScaledObject, error) {
	one := int32(1)
	thirty := int32(30)

	scaledObjectLabels := makeLabels(proc)
	scaledObjectLabels["deploymentName"] = deployment.Name

	scaledObject := &v1alpha1.ScaledObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:            names.ScaledObject(proc),
			Namespace:       proc.Namespace,
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(proc)},
			Labels:          scaledObjectLabels,
		},
		Spec: v1alpha1.ScaledObjectSpec{
			ScaleTargetRef: v1alpha1.ObjectReference{
				DeploymentName: deployment.Name,
			},
			PollingInterval: &one,
			CooldownPeriod:  &thirty,
			Triggers:        triggers(proc),
		},
	}

	return scaledObject, nil
}

func triggers(proc *streamingv1alpha1.Processor) []v1alpha1.ScaleTriggers {
	result := make([]v1alpha1.ScaleTriggers, len(proc.Status.InputAddresses))
	for i, topic := range proc.Status.InputAddresses {
		result[i].Type = "liiklus"
		result[i].Metadata = map[string]string{
			"address": strings.Split(topic, "/")[0],
			"group":   proc.Name,
			"topic":   strings.Split(topic, "/")[1],
		}
	}
	return result
}
