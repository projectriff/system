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
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	projectriffv1alpha1 "github.com/projectriff/system/pkg/apis/projectriff/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler/v1alpha1/application/resources/names"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeConfiguration creates a Configuration from a Application object.
func MakeConfiguration(application *projectriffv1alpha1.Application) (*servingv1alpha1.Configuration, error) {
	configuration := &servingv1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Configuration(application),
			Namespace: application.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(application),
			},
			Labels: makeLabels(application),
		},
		Spec: servingv1alpha1.ConfigurationSpec{
			Build: makeConfigurationBuild(application),
			RevisionTemplate: servingv1alpha1.RevisionTemplateSpec{
				Spec: servingv1alpha1.RevisionSpec{
					Container: corev1.Container{
						Image:     application.Spec.Image,
						EnvFrom:   application.Spec.Run.EnvFrom,
						Env:       application.Spec.Run.Env,
						Resources: application.Spec.Run.Resources,
					},
				},
			},
		},
	}

	return configuration, nil
}

func makeConfigurationBuild(application *projectriffv1alpha1.Application) *servingv1alpha1.RawExtension {
	if application.Spec.Build.Source == nil {
		return nil
	}
	return &servingv1alpha1.RawExtension{
		Object: makeBuild(application),
	}
}

func makeBuild(application *projectriffv1alpha1.Application) *buildv1alpha1.Build {
	return &buildv1alpha1.Build{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "build.knative.dev/v1alpha1",
			Kind:       "Build",
		},
		Spec: buildv1alpha1.BuildSpec{
			ServiceAccountName: "riff-build",
			Source:             makeBuildSource(application),
			Template: &buildv1alpha1.TemplateInstantiationSpec{
				Name:      application.Spec.Build.Template,
				Kind:      "ClusterBuildTemplate",
				Arguments: makeBuildArguments(application),
			},
			Volumes: makeBuildVolumes(application),
		},
	}
}

func makeBuildSource(application *projectriffv1alpha1.Application) *buildv1alpha1.SourceSpec {
	as := application.Spec.Build.Source
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

func makeBuildArguments(application *projectriffv1alpha1.Application) []buildv1alpha1.ArgumentSpec {
	args := []buildv1alpha1.ArgumentSpec{
		{Name: "IMAGE", Value: application.Spec.Image},
	}
	if application.Status.BuildCacheName != "" {
		args = append(args, buildv1alpha1.ArgumentSpec{Name: "CACHE", Value: "cache"})
	}
	for _, arg := range application.Spec.Build.Arguments {
		args = append(args, buildv1alpha1.ArgumentSpec{Name: arg.Name, Value: arg.Value})
	}
	return args
}

func makeBuildVolumes(application *projectriffv1alpha1.Application) []corev1.Volume {
	volumes := []corev1.Volume{}
	if application.Status.BuildCacheName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "cache",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: application.Status.BuildCacheName},
			},
		})
	}
	return volumes
}
