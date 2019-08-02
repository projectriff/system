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
	knapis "github.com/knative/pkg/apis"
)

const (
	ImageConditionReady                              = knapis.ConditionReady
	ImageConditionImageResolved knapis.ConditionType = "ImageResolved"
)

var containerCondSet = knapis.NewLivingConditionSet(
	ImageConditionImageResolved,
)

func (is *ImageStatus) GetObservedGeneration() int64 {
	return is.ObservedGeneration
}

func (is *ImageStatus) IsReady() bool {
	return containerCondSet.Manage(is).IsHappy()
}

func (*ImageStatus) GetReadyConditionType() knapis.ConditionType {
	return ImageConditionReady
}

func (is *ImageStatus) GetCondition(t knapis.ConditionType) *knapis.Condition {
	return containerCondSet.Manage(is).GetCondition(t)
}

func (is *ImageStatus) InitializeConditions() {
	containerCondSet.Manage(is).InitializeConditions()
}

func (is *ImageStatus) MarkImageDefaultPrefixMissing(message string) {
	containerCondSet.Manage(is).MarkFalse(ImageConditionImageResolved, "DefaultImagePrefixMissing", message)
}

func (is *ImageStatus) MarkImageInvalid(message string) {
	containerCondSet.Manage(is).MarkFalse(ImageConditionImageResolved, "ImageInvalid", message)
}

func (is *ImageStatus) MarkImageMissing(message string) {
	containerCondSet.Manage(is).MarkFalse(ImageConditionImageResolved, "ImageMissing", message)
}

func (is *ImageStatus) MarkImageResolved() {
	containerCondSet.Manage(is).MarkTrue(ImageConditionImageResolved)
}
