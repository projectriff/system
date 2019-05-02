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
	runv1alpha1 "github.com/projectriff/system/pkg/apis/run/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/requestprocessor/resources/names"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeRoute creates a Route from an RequestProcessor object.
func MakeRoute(rp *runv1alpha1.RequestProcessor) (*knservingv1alpha1.Route, error) {
	if len(rp.Spec) != len(rp.Status.ConfigurationNames) {
		return nil, fmt.Errorf("Unable to create Route, wanted %d reconciled configurations, found %d", len(rp.Spec), len(rp.Status.ConfigurationNames))
	}

	route := &knservingv1alpha1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Route(rp),
			Namespace: rp.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(rp),
			},
			Labels: makeLabels(rp),
		},
		Spec: knservingv1alpha1.RouteSpec{
			Traffic: makeRouteTraffic(rp),
		},
	}

	return route, nil
}

func makeRouteTraffic(rp *runv1alpha1.RequestProcessor) []knservingv1alpha1.TrafficTarget {
	traffic := []knservingv1alpha1.TrafficTarget{}

	for i, rpsi := range rp.Spec {
		traffic = append(traffic, knservingv1alpha1.TrafficTarget{
			Name:              rpsi.Name,
			Percent:           *rpsi.Percent,
			ConfigurationName: rp.Status.ConfigurationNames[i],
		})
	}

	return traffic
}
