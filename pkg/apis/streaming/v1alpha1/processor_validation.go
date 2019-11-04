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
	"k8s.io/apimachinery/pkg/api/equality"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/projectriff/system/pkg/validation"
)

// +kubebuilder:webhook:path=/validate-streaming-projectriff-io-v1alpha1-processor,mutating=false,failurePolicy=fail,groups=streaming.projectriff.io,resources=processors,verbs=create;update,versions=v1alpha1,name=processors.streaming.projectriff.io

var (
	_ webhook.Validator         = &Processor{}
	_ validation.FieldValidator = &Processor{}
)

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Processor) ValidateCreate() error {
	return r.Validate().ToAggregate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Processor) ValidateUpdate(old runtime.Object) error {
	// TODO check for immutable fields
	return r.Validate().ToAggregate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Processor) ValidateDelete() error {
	return nil
}

func (r *Processor) Validate() validation.FieldErrors {
	errs := validation.FieldErrors{}

	errs = errs.Also(r.Spec.Validate().ViaField("spec"))

	return errs
}

func (s *ProcessorSpec) Validate() validation.FieldErrors {
	if equality.Semantic.DeepEqual(s, &ProcessorSpec{}) {
		return validation.ErrMissingField(validation.CurrentField)
	}

	errs := validation.FieldErrors{}

	if s.FunctionRef == "" {
		errs = errs.Also(validation.ErrMissingField("functionRef"))
	}

	// at least one input is required
	if len(s.Inputs) == 0 {
		errs = errs.Also(validation.ErrMissingField("inputs"))
	}
	for i, input := range s.Inputs {
		if input == "" {
			errs = errs.Also(validation.ErrInvalidArrayValue(input, "inputs", i))
		}
	}

	// If present, inputNames must have the same length as inputs
	if len(s.InputNames) != 0 && len(s.InputNames) != len(s.Inputs) {
		errs = errs.Also(validation.FieldErrors{
			field.Invalid(field.NewPath("inputNames"), s.InputNames, "when set, inputNames must have the same length as inputs"),
		})
	}

	// If present, outputNames must have the same length as outputs
	if len(s.OutputNames) != 0 && len(s.OutputNames) != len(s.Outputs) {
		errs = errs.Also(validation.FieldErrors{
			field.Invalid(field.NewPath("outputNames"), s.OutputNames, "when set, outputNames must have the same length as outputs"),
		})
	}

	// outputs are optional
	for i, output := range s.Outputs {
		if output == "" {
			errs = errs.Also(validation.ErrInvalidArrayValue(output, "outputs", i))
		}
	}

	return errs
}
