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
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/application/resources/names"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeBuild creates a Build from an Application object.
func MakeBuild(a *buildv1alpha1.Application) (*knbuildv1alpha1.Build, error) {
	if a.Spec.Source == nil {
		return nil, nil
	}

	build := &knbuildv1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Build(a),
			Namespace: a.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(a),
			},
			Labels: makeLabels(a),
		},
		Spec: knbuildv1alpha1.BuildSpec{
			ServiceAccountName: "riff-build",
			Source:             makeBuildSource(a),
			Template: &knbuildv1alpha1.TemplateInstantiationSpec{
				Name:      "riff-application",
				Kind:      "ClusterBuildTemplate",
				Arguments: makeBuildArguments(a),
			},
			Volumes: makeBuildVolumes(a),
		},
	}

	return build, nil
}

func makeBuildSource(a *buildv1alpha1.Application) *knbuildv1alpha1.SourceSpec {
	as := a.Spec.Source
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

func makeBuildArguments(a *buildv1alpha1.Application) []knbuildv1alpha1.ArgumentSpec {
	args := []knbuildv1alpha1.ArgumentSpec{
		{Name: "IMAGE", Value: a.Status.TargetImage},
	}
	if a.Status.BuildCacheName != "" {
		args = append(args, knbuildv1alpha1.ArgumentSpec{Name: "CACHE", Value: "cache"})
	}
	return args
}

func makeBuildVolumes(a *buildv1alpha1.Application) []corev1.Volume {
	volumes := []corev1.Volume{}
	if a.Status.BuildCacheName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "cache",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: a.Status.BuildCacheName},
			},
		})
	}
	return volumes
}
