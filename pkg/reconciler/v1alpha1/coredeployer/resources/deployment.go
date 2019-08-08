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
	"github.com/projectriff/system/pkg/apis/core"
	corev1alpha1 "github.com/projectriff/system/pkg/apis/core/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/coredeployer/resources/names"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeDeployment creates a Deployment from a Deployer object.
func MakeDeployment(d *corev1alpha1.Deployer) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Deployment(d),
			Namespace: d.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(d),
			},
			Labels: makeLabels(d),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					core.DeployerLabelKey: d.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: makeLabels(d),
				},
				Spec: *d.Spec.Template,
			},
		},
	}

	return deployment, nil
}
