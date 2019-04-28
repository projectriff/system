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

	"github.com/knative/pkg/logging"
	corev1 "k8s.io/api/core/v1"
)

func (rp *RequestProcessor) SetDefaults(ctx context.Context) {
	rp.Spec.SetDefaults(ctx)
}

func (rps RequestProcessorSpec) SetDefaults(ctx context.Context) {
	logger := logging.FromContext(ctx)

	allocatedTraffic := 0
	missingTraffic := []int{}
	for i := range rps {
		if rps[i].Percent != nil {
			allocatedTraffic = allocatedTraffic + *rps[i].Percent
		} else {
			missingTraffic = append(missingTraffic, i)
		}

		rps[i].SetDefaults(ctx)
	}

	// traffic must sum to 100 percent
	if allocatedTraffic < 100 && len(missingTraffic) > 0 {
		share := (100 - allocatedTraffic) / len(missingTraffic)
		remainder := 100 - allocatedTraffic - share*len(missingTraffic)
		logger.Debugf("%d%% traffic allocated over %d items, %d items unallocated to receive %d%% each with %d%% remaining\n", allocatedTraffic, len(rps)-len(missingTraffic), len(missingTraffic), share, remainder)
		for i, missingTrafficIdx := range missingTraffic {
			pct := share
			if i < remainder {
				pct++
			}
			rps[missingTrafficIdx].Percent = &pct
		}
	}
}

func (rpsi *RequestProcessorSpecItem) SetDefaults(ctx context.Context) {
	if len(rpsi.Containers) == 0 {
		rpsi.PodSpec.Containers = append(rpsi.Containers, corev1.Container{})
	}
}
