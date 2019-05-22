/*
Copyright 2018 The Knative Authors

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

package resources

import (
	"fmt"

	"github.com/knative/pkg/kmeta"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	requestv1alpha1 "github.com/projectriff/system/pkg/apis/request/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/route/resources/names"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeKnRoute creates a KnRoute from a Route object.
func MakeKnRoute(r *requestv1alpha1.Route) (*knservingv1alpha1.Route, error) {
	if r.Status.KnConfigurationName == "" {
		return nil, fmt.Errorf("unable to create Route, waiting for Configuration")
	}

	knroute := &knservingv1alpha1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.KnRoute(r),
			Namespace: r.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(r),
			},
			Labels: makeLabels(r),
		},
		Spec: knservingv1alpha1.RouteSpec{
			Traffic: []knservingv1alpha1.TrafficTarget{
				knservingv1alpha1.TrafficTarget{
					Percent:           100,
					ConfigurationName: r.Status.KnConfigurationName,
				},
			},
		},
	}

	return knroute, nil
}
