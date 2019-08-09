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

	corev1 "k8s.io/api/core/v1"
)

func (c *Deployer) SetDefaults(ctx context.Context) {
	c.Spec.SetDefaults(ctx)
}

func (cs *DeployerSpec) SetDefaults(ctx context.Context) {
	if cs.Template == nil {
		cs.Template = &corev1.PodSpec{}
	}
	if len(cs.Template.Containers) == 0 {
		cs.Template.Containers = append(cs.Template.Containers, corev1.Container{})
	}
}
