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
	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
)

// makeLabels constructs the labels we will apply to Processor resource.
func makeLabels(p *streamingv1alpha1.Processor) map[string]string {
	labels := make(map[string]string, len(p.ObjectMeta.Labels)+1)
	labels[streamingv1alpha1.ProcessorLabelKey] = p.Name

	// Pass through the labels on the Processor to child resources.
	for k, v := range p.ObjectMeta.Labels {
		labels[k] = v
	}
	return labels
}
