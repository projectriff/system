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
	systemapis "github.com/projectriff/system/pkg/apis"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
)

func (c *Deployer) Validate(ctx context.Context) *apis.FieldError {
	errs := &apis.FieldError{}
	errs = errs.Also(systemapis.ValidateObjectMetadata(c.GetObjectMeta()).ViaField("metadata"))
	errs = errs.Also(c.Spec.Validate(ctx).ViaField("spec"))
	return errs
}

func (cs DeployerSpec) Validate(ctx context.Context) *apis.FieldError {
	if equality.Semantic.DeepEqual(cs, DeployerSpec{}) {
		return apis.ErrMissingField(apis.CurrentField)
	}

	errs := &apis.FieldError{}

	if diff := cmp.Diff(&corev1.PodSpec{
		// add supported PodSpec fields here, otherwise their usage will be rejected
		ServiceAccountName: cs.Template.ServiceAccountName,
		// the defaulter guarantees at least one container
		Containers: filterInvalidContainers(cs.Template.Containers[:1]),
		Volumes:    filterInvalidVolumes(cs.Template.Volumes),
	}, cs.Template); diff != "" {
		err := apis.ErrDisallowedFields(apis.CurrentField)
		err.Details = fmt.Sprintf("limited Template fields may be set (-want, +got) = %v", diff)
		errs = errs.Also(err)
	}

	if cs.Build == nil && cs.Template.Containers[0].Image == "" {
		errs = errs.Also(apis.ErrMissingOneOf("build", "template.containers[0].image"))
	} else if cs.Build != nil && cs.Template.Containers[0].Image != "" {
		errs = errs.Also(apis.ErrMultipleOneOf("build", "template.containers[0].image"))
	} else if cs.Build != nil {
		errs = errs.Also(cs.Build.Validate(ctx).ViaField("build"))
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
