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

func (d *Deployer) SetDefaults(ctx context.Context) {
	d.Spec.SetDefaults(ctx)
}

func (ds *DeployerSpec) SetDefaults(ctx context.Context) {
	if ds.Template == nil {
		ds.Template = &corev1.PodSpec{}
	}
	if len(ds.Template.Containers) == 0 {
		ds.Template.Containers = append(ds.Template.Containers, corev1.Container{})
	}
	if ds.Template.Containers[0].Name == "" {
		ds.Template.Containers[0].Name = "handler"
	}
}
