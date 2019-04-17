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
	planv1alpha1 "github.com/projectriff/system/pkg/apis/plan/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/plan/resources/names"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeRunPlan creates a RunPlan from a Plan object.
func MakeRunPlan(plan *planv1alpha1.Plan) (*planv1alpha1.RunPlan, error) {
	if plan.Spec.Run == nil {
		// no run was requested
		return nil, nil
	}

	runplan := &planv1alpha1.RunPlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.BuildPlan(plan),
			Namespace: plan.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(plan),
			},
			Labels: makeLabels(plan),
		},
		Spec: planv1alpha1.RunPlanSpec{
			Image: plan.Status.LatestImage,
			Run:   *plan.Spec.Run,
		},
	}

	return runplan, nil
}
