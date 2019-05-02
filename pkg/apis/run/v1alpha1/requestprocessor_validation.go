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
 */

package v1alpha1

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/apis"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
)

func (rp *RequestProcessor) Validate(ctx context.Context) *apis.FieldError {
	errs := &apis.FieldError{}
	errs = errs.Also(validateObjectMetadata(rp.GetObjectMeta()).ViaField("metadata"))
	errs = errs.Also(rp.Spec.Validate(ctx).ViaField("spec"))
	return errs
}

func (rps RequestProcessorSpec) Validate(ctx context.Context) *apis.FieldError {
	if equality.Semantic.DeepEqual(rps, RequestProcessorSpec{}) {
		return apis.ErrMissingField(apis.CurrentField)
	}

	errs := &apis.FieldError{}

	seenNames := map[string]int{}
	for i, rpsi := range rps {
		errs = errs.Also(rpsi.Validate(ctx).ViaIndex(i))

		if rpsi.Name == "" {
			errs = errs.Also(apis.ErrMissingField("name").ViaIndex(i))
		}

		// require names be unique
		if j, ok := seenNames[rpsi.Name]; ok {
			errs = errs.Also(&apis.FieldError{
				Message: fmt.Sprintf("duplicate name %q", rpsi.Name),
				Paths: []string{
					fmt.Sprintf("[%d].name", j),
					fmt.Sprintf("[%d].name", i),
				},
			})
		} else {
			seenNames[rpsi.Name] = i
		}
	}

	return errs
}

func (rpsi *RequestProcessorSpecItem) Validate(ctx context.Context) *apis.FieldError {
	if equality.Semantic.DeepEqual(rpsi, &RequestProcessorSpecItem{}) {
		return apis.ErrMissingField(apis.CurrentField)
	}

	errs := &apis.FieldError{}

	if diff := cmp.Diff(&corev1.PodSpec{
		// add supported PodSpec fields here, otherwise their usage will be rejected
		ServiceAccountName: rpsi.Template.ServiceAccountName,
		// the defaulter guarantees at least one container
		Containers: filterInvalidContainers(rpsi.Template.Containers[:1]),
		Volumes:    filterInvalidVolumes(rpsi.Template.Volumes),
	}, rpsi.Template); diff != "" {
		err := apis.ErrDisallowedFields(apis.CurrentField)
		err.Details = fmt.Sprintf("limited Template fields may be set (-want, +got) = %v", diff)
		errs = errs.Also(err)
	}

	if rpsi.Build == nil && rpsi.Template.Containers[0].Image == "" {
		errs = errs.Also(apis.ErrMissingOneOf("build", "template.containers[0].image"))
	} else if rpsi.Build != nil && rpsi.Template.Containers[0].Image != "" {
		errs = errs.Also(apis.ErrMultipleOneOf("build", "template.containers[0].image"))
	} else if rpsi.Build != nil {
		errs = errs.Also(rpsi.Build.Validate(ctx).ViaField("build"))
	}

	return errs
}

func (b *Build) Validate(ctx context.Context) *apis.FieldError {
	if equality.Semantic.DeepEqual(b, &Build{}) {
		return apis.ErrMissingField(apis.CurrentField)
	}

	errs := &apis.FieldError{}
	used := []string{}
	unused := []string{}

	if b.Application != nil {
		used = append(used, "application")
		errs = errs.Also(b.Application.Validate(ctx).ViaField("application"))
	} else {
		unused = append(unused, "application")
	}

	if b.ApplicationRef != "" {
		used = append(used, "applicationRef")
	} else {
		unused = append(unused, "applicationRef")
	}

	if b.Function != nil {
		used = append(used, "function")
		errs = errs.Also(b.Function.Validate(ctx).ViaField("function"))
	} else {
		unused = append(unused, "function")
	}

	if b.FunctionRef != "" {
		used = append(used, "functionRef")
	} else {
		unused = append(unused, "functionRef")
	}

	if len(used) == 0 {
		errs = errs.Also(apis.ErrMissingOneOf(unused...))
	} else if len(used) > 1 {
		errs = errs.Also(apis.ErrMultipleOneOf(used...))
	}

	return errs
}

func filterInvalidContainers(containers []corev1.Container) []corev1.Container {
	// TODO remove unsupported fields
	return containers
}

func filterInvalidVolumes(volumes []corev1.Volume) []corev1.Volume {
	// TODO remove unsupported fields
	return volumes
}
