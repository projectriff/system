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
	"github.com/knative/pkg/kmeta"
	projectriffv1alpha1 "github.com/projectriff/system/pkg/apis/projectriff/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/function/resources/names"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeApplication creates a Application from a Function object.
func MakeApplication(function *projectriffv1alpha1.Function) (*projectriffv1alpha1.Application, error) {
	a := &projectriffv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Application(function),
			Namespace: function.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(function),
			},
			Labels: makeLabels(function),
		},
		Spec: projectriffv1alpha1.ApplicationSpec{
			Image: function.Spec.Image,
			Build: projectriffv1alpha1.ApplicationBuild{
				Template: "riff-cnb",
				Arguments: []projectriffv1alpha1.BuildArgument{
					{Name: "FUNCTION_ARTIFACT", Value: function.Spec.Build.Artifact},
					{Name: "FUNCTION_HANDLER", Value: function.Spec.Build.Handler},
					{Name: "FUNCTION_LANGUAGE", Value: function.Spec.Build.Invoker},
				},
				CacheSize: function.Spec.Build.CacheSize,
				Source:    function.Spec.Build.Source,
			},
			Run: projectriffv1alpha1.ApplicationRun{
				EnvFrom:   function.Spec.Run.EnvFrom,
				Env:       function.Spec.Run.Env,
				Resources: function.Spec.Run.Resources,
			},
		},
	}

	return a, nil
}
