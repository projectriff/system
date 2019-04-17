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
	"github.com/projectriff/system/pkg/apis/plan"
	planv1alpha1 "github.com/projectriff/system/pkg/apis/plan/v1alpha1"
)

// makeLabels constructs the labels we will apply for RunPlan resources.
func makeLabels(a *planv1alpha1.RunPlan) map[string]string {
	labels := make(map[string]string, len(a.ObjectMeta.Labels)+1)
	labels[plan.RunPlanLabelKey] = a.Name

	// Pass through the labels on the Application to child resources.
	for k, v := range a.ObjectMeta.Labels {
		labels[k] = v
	}
	return labels
}
