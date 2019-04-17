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
	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	"github.com/knative/pkg/kmeta"
	planv1alpha1 "github.com/projectriff/system/pkg/apis/plan/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/buildplan/resources/names"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeBuild creates a Build from an BuildPlan object.
func MakeBuild(buildplan *planv1alpha1.BuildPlan) (*buildv1alpha1.Build, error) {
	if buildplan.Spec.Build.Source == nil {
		// no build was requested
		return nil, nil
	}

	build := &buildv1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Build(buildplan),
			Namespace: buildplan.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(buildplan),
			},
			Labels: makeLabels(buildplan),
		},
		Spec: buildv1alpha1.BuildSpec{
			ServiceAccountName: "riff-build",
			Source:             makeBuildSource(buildplan),
			Template: &buildv1alpha1.TemplateInstantiationSpec{
				Name:      buildplan.Spec.Build.Template,
				Kind:      "ClusterBuildTemplate",
				Arguments: makeBuildArguments(buildplan),
			},
			Volumes: makeBuildVolumes(buildplan),
		},
	}

	return build, nil
}

func makeBuildSource(buildplan *planv1alpha1.BuildPlan) *buildv1alpha1.SourceSpec {
	as := buildplan.Spec.Build.Source
	s := &buildv1alpha1.SourceSpec{
		SubPath: as.SubPath,
	}
	if as.Git != nil {
		s.Git = &buildv1alpha1.GitSourceSpec{
			Url:      as.Git.URL,
			Revision: as.Git.Revision,
		}
	}
	return s
}

func makeBuildArguments(buildplan *planv1alpha1.BuildPlan) []buildv1alpha1.ArgumentSpec {
	args := []buildv1alpha1.ArgumentSpec{
		{Name: "IMAGE", Value: buildplan.Spec.Image},
	}
	if buildplan.Status.BuildCacheName != "" {
		args = append(args, buildv1alpha1.ArgumentSpec{Name: "CACHE", Value: "cache"})
	}
	for _, arg := range buildplan.Spec.Build.Arguments {
		args = append(args, buildv1alpha1.ArgumentSpec{Name: arg.Name, Value: arg.Value})
	}
	return args
}

func makeBuildVolumes(buildplan *planv1alpha1.BuildPlan) []corev1.Volume {
	volumes := []corev1.Volume{}
	if buildplan.Status.BuildCacheName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "cache",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: buildplan.Status.BuildCacheName},
			},
		})
	}
	return volumes
}
