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

package v1alpha1

import (
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// +kubebuilder:webhook:path=/mutate-streaming-projectriff-io-v1alpha1-stream,mutating=true,failurePolicy=fail,sideEffects=none,admissionReviewVersions=v1beta1,groups=streaming.projectriff.io,resources=streams,verbs=create;update,versions=v1alpha1,name=streams.streaming.projectriff.io

var _ webhook.Defaulter = &Stream{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Stream) Default() {
	r.Spec.Default()
}

func (s *StreamSpec) Default() {
	if s.ContentType == "" {
		s.ContentType = "application/octet-stream"
	}
}
