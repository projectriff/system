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
	commonv1alpha1 "github.com/projectriff/system/pkg/apis/common/v1alpha1"
	planv1alpha1 "github.com/projectriff/system/pkg/apis/plan/v1alpha1"
	projectriffv1alpha1 "github.com/projectriff/system/pkg/apis/projectriff/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/application/resources/names"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakePlan creates a Plan from a Application object.
func MakePlan(application *projectriffv1alpha1.Application) (*planv1alpha1.Plan, error) {
	plan := &planv1alpha1.Plan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Plan(application),
			Namespace: application.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(application),
			},
			Labels: makeLabels(application),
		},
		Spec: planv1alpha1.PlanSpec{
			Image: application.Spec.Image,
			Run:   application.Spec.Run,
		},
	}

	if application.Spec.Build != nil {
		plan.Spec.Build = &commonv1alpha1.Build{
			Template:  "cf-cnb",
			Arguments: []commonv1alpha1.BuildArgument{},
			CacheSize: application.Spec.Build.CacheSize,
			Source:    application.Spec.Build.Source,
		}
	}

	return plan, nil
}
