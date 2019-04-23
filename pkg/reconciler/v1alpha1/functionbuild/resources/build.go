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
	knbuildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	"github.com/knative/pkg/kmeta"
	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/functionbuild/resources/names"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeBuild creates a Build from an FunctionBuild object.
func MakeBuild(fb *buildv1alpha1.FunctionBuild) (*knbuildv1alpha1.Build, error) {
	build := &knbuildv1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Build(fb),
			Namespace: fb.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(fb),
			},
			Labels: makeLabels(fb),
		},
		Spec: knbuildv1alpha1.BuildSpec{
			ServiceAccountName: "riff-build",
			Source:             makeBuildSource(fb),
			Template: &knbuildv1alpha1.TemplateInstantiationSpec{
				Name:      "riff-cnb",
				Kind:      "ClusterBuildTemplate",
				Arguments: makeBuildArguments(fb),
			},
			Volumes: makeBuildVolumes(fb),
		},
	}

	return build, nil
}

func makeBuildSource(fb *buildv1alpha1.FunctionBuild) *knbuildv1alpha1.SourceSpec {
	as := fb.Spec.Source
	s := &knbuildv1alpha1.SourceSpec{
		SubPath: as.SubPath,
	}
	if as.Git != nil {
		s.Git = &knbuildv1alpha1.GitSourceSpec{
			Url:      as.Git.URL,
			Revision: as.Git.Revision,
		}
	}
	return s
}

func makeBuildArguments(fb *buildv1alpha1.FunctionBuild) []knbuildv1alpha1.ArgumentSpec {
	args := []knbuildv1alpha1.ArgumentSpec{
		{Name: "IMAGE", Value: fb.Spec.Image},
		{Name: "FUNCTION_ARTIFACT", Value: fb.Spec.Artifact},
		{Name: "FUNCTION_HANDLER", Value: fb.Spec.Handler},
		{Name: "FUNCTION_LANGUAGE", Value: fb.Spec.Invoker},
	}
	if fb.Status.BuildCacheName != "" {
		args = append(args, knbuildv1alpha1.ArgumentSpec{Name: "CACHE", Value: "cache"})
	}
	return args
}

func makeBuildVolumes(fb *buildv1alpha1.FunctionBuild) []corev1.Volume {
	volumes := []corev1.Volume{}
	if fb.Status.BuildCacheName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "cache",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: fb.Status.BuildCacheName},
			},
		})
	}
	return volumes
}
