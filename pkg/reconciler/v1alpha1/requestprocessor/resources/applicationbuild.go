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
	runv1alpha1 "github.com/projectriff/system/pkg/apis/run/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/requestprocessor/resources/names"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeApplicationBuild creates a ApplicationBuild from an RequestProcessor object.
func MakeApplicationBuild(rp *runv1alpha1.RequestProcessor, i int) (*buildv1alpha1.ApplicationBuild, error) {
	if rp.Spec[i].Build.Application == nil {
		return nil, nil
	}

	ApplicationBuild := &buildv1alpha1.ApplicationBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.TagOrIndex(rp, i),
			Namespace: rp.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(rp),
			},
			Labels: makeLabels(rp),
		},
		Spec: *rp.Spec[i].Build.Application,
	}

	return ApplicationBuild, nil
}
