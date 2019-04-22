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
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/applicationbuild/resources/names"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeInternalBuild creates a InternalBuild from an ApplicationBuild object.
func MakeInternalBuild(ab *buildv1alpha1.ApplicationBuild) (*buildv1alpha1.InternalBuild, error) {
	ib := &buildv1alpha1.InternalBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.InternalBuild(ab),
			Namespace: ab.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(ab),
			},
			Labels: makeLabels(ab),
		},
		Spec: buildv1alpha1.InternalBuildSpec{
			Image:     ab.Spec.Image,
			CacheSize: ab.Spec.CacheSize,
			Template:  "cf-cnb",
			Source:    ab.Spec.Source,
			Arguments: []buildv1alpha1.BuildArgument{},
		},
	}

	return ib, nil
}
