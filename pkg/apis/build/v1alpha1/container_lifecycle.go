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
	ContainerConditionReady                              = knapis.ConditionReady
	ContainerConditionImageResolved knapis.ConditionType = "ImageResolved"
)

var containerCondSet = knapis.NewLivingConditionSet(
	ContainerConditionImageResolved,
)

func (cs *ContainerStatus) GetObservedGeneration() int64 {
	return cs.ObservedGeneration
}

func (cs *ContainerStatus) IsReady() bool {
	return containerCondSet.Manage(cs).IsHappy()
}

func (*ContainerStatus) GetReadyConditionType() knapis.ConditionType {
	return ContainerConditionReady
}

func (cs *ContainerStatus) GetCondition(t knapis.ConditionType) *knapis.Condition {
	return containerCondSet.Manage(cs).GetCondition(t)
}

func (cs *ContainerStatus) InitializeConditions() {
	containerCondSet.Manage(cs).InitializeConditions()
}

func (cs *ContainerStatus) MarkImageDefaultPrefixMissing(message string) {
	containerCondSet.Manage(cs).MarkFalse(ContainerConditionImageResolved, "DefaultImagePrefixMissing", message)
}

func (cs *ContainerStatus) MarkImageInvalid(message string) {
	containerCondSet.Manage(cs).MarkFalse(ContainerConditionImageResolved, "ImageInvalid", message)
}

func (cs *ContainerStatus) MarkImageMissing(message string) {
	containerCondSet.Manage(cs).MarkFalse(ContainerConditionImageResolved, "ImageMissing", message)
}

func (cs *ContainerStatus) MarkImageResolved() {
	containerCondSet.Manage(cs).MarkTrue(ContainerConditionImageResolved)
}
