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
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/functionbuild/resources/names"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeInternalBuild creates a InternalBuild from an FunctionBuild object.
func MakeInternalBuild(fb *buildv1alpha1.FunctionBuild) (*buildv1alpha1.InternalBuild, error) {
	ib := &buildv1alpha1.InternalBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.InternalBuild(fb),
			Namespace: fb.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(fb),
			},
			Labels: makeLabels(fb),
		},
		Spec: buildv1alpha1.InternalBuildSpec{
			Image:     fb.Spec.Image,
			CacheSize: fb.Spec.CacheSize,
			Template:  "riff-cnb",
			Source:    fb.Spec.Source,
			Arguments: []buildv1alpha1.BuildArgument{
				{Name: "FUNCTION_ARTIFACT", Value: fb.Spec.Artifact},
				{Name: "FUNCTION_HANDLER", Value: fb.Spec.Handler},
				{Name: "FUNCTION_LANGUAGE", Value: fb.Spec.Invoker},
			},
		},
	}

	return ib, nil
}
