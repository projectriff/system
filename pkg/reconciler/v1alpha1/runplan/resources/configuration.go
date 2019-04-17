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
	"github.com/knative/pkg/kmeta"
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	planv1alpha1 "github.com/projectriff/system/pkg/apis/plan/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/runplan/resources/names"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeConfiguration creates a Configuration from a RunPlan object.
func MakeConfiguration(runplan *planv1alpha1.RunPlan) (*servingv1alpha1.Configuration, error) {
	configuration := &servingv1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Configuration(runplan),
			Namespace: runplan.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(runplan),
			},
			Labels: makeLabels(runplan),
		},
		Spec: servingv1alpha1.ConfigurationSpec{
			RevisionTemplate: servingv1alpha1.RevisionTemplateSpec{
				Spec: servingv1alpha1.RevisionSpec{
					Container: corev1.Container{
						Image:     runplan.Spec.Image,
						EnvFrom:   runplan.Spec.Run.EnvFrom,
						Env:       runplan.Spec.Run.Env,
						Resources: runplan.Spec.Run.Resources,
					},
				},
			},
		},
	}

	return configuration, nil
}
