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
	corev1 "k8s.io/api/core/v1"
)

func makePodSpec(containers ...corev1.Container) corev1.PodSpec {
	return corev1.PodSpec{
		Containers: containers,
	}
}

func makeDefaultPodSpec() corev1.PodSpec {
	return makePodSpec(corev1.Container{})
}

func makePint(i int) *int {
	return &i
}
