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
	"github.com/projectriff/system/pkg/apis/knative"
	knativev1alpha1 "github.com/projectriff/system/pkg/apis/knative/v1alpha1"
)

// makeLabels constructs the labels we will apply to Deployer resource.
func makeLabels(c *knativev1alpha1.Deployer) map[string]string {
	labels := make(map[string]string, len(c.ObjectMeta.Labels)+1)
	labels[knative.DeployerLabelKey] = c.Name

	// Pass through the labels on the Function to child resources.
	for k, v := range c.ObjectMeta.Labels {
		labels[k] = v
	}
	return labels
}
