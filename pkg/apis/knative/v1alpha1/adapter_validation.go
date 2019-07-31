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

	"github.com/knative/pkg/apis"
	systemapis "github.com/projectriff/system/pkg/apis"
	"k8s.io/apimachinery/pkg/api/equality"
)

func (a *Adapter) Validate(ctx context.Context) *apis.FieldError {
	errs := &apis.FieldError{}
	errs = errs.Also(systemapis.ValidateObjectMetadata(a.GetObjectMeta()).ViaField("metadata"))
	errs = errs.Also(a.Spec.Validate(ctx).ViaField("spec"))
	return errs
}

func (as AdapterSpec) Validate(ctx context.Context) *apis.FieldError {
	if equality.Semantic.DeepEqual(as, AdapterSpec{}) {
		return apis.ErrMissingField(apis.CurrentField)
	}

	errs := &apis.FieldError{}

	errs = errs.Also(as.Build.Validate(ctx).ViaField("build"))
	errs = errs.Also(as.Target.Validate(ctx).ViaField("artget"))

	return errs
}

func (t *Target) Validate(ctx context.Context) *apis.FieldError {
	if equality.Semantic.DeepEqual(t, &Target{}) {
		return apis.ErrMissingField(apis.CurrentField)
	}

	errs := &apis.FieldError{}
	used := []string{}
	unused := []string{}

	if t.ServiceRef != "" {
		used = append(used, "serviceRef")
	} else {
		unused = append(unused, "serviceRef")
	}

	if t.ConfigurationRef != "" {
		used = append(used, "configurationRef")
	} else {
		unused = append(unused, "configurationRef")
	}

	if len(used) == 0 {
		errs = errs.Also(apis.ErrMissingOneOf(unused...))
	} else if len(used) > 1 {
		errs = errs.Also(apis.ErrMultipleOneOf(used...))
	}

	return errs
}
