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
	knservingv1beta1 "github.com/knative/serving/pkg/apis/serving/v1beta1"
	knativev1alpha1 "github.com/projectriff/system/pkg/apis/knative/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/knativehandler/resources/names"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeRoute creates a Route from a Handler object.
func MakeRoute(h *knativev1alpha1.Handler) (*knservingv1alpha1.Route, error) {
	if h.Status.ConfigurationName == "" {
		return nil, fmt.Errorf("unable to create Route, waiting for Configuration")
	}

	route := &knservingv1alpha1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Route(h),
			Namespace: h.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(h),
			},
			Labels: makeLabels(h),
		},
		Spec: knservingv1alpha1.RouteSpec{
			Traffic: []knservingv1alpha1.TrafficTarget{
				{
					TrafficTarget: knservingv1beta1.TrafficTarget{
						Percent:           100,
						ConfigurationName: h.Status.ConfigurationName,
					},
				},
			},
		},
	}

	return route, nil
}
