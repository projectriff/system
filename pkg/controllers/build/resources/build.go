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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	knbuildv1alpha1 "github.com/projectriff/system/pkg/apis/thirdparty/knative/build/v1alpha1"
	"github.com/projectriff/system/pkg/controllers/build/resources/names"
)

// MakeFunctionBuild creates a Build from an Function object.
func MakeFunctionBuild(f *buildv1alpha1.Function) *knbuildv1alpha1.Build {
	if f.Spec.Source == nil {
		return nil
	}

	build := &knbuildv1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.FunctionBuild(f),
			Namespace: f.Namespace,
			Labels:    makeFunctionLabels(f),
		},
		Spec: knbuildv1alpha1.BuildSpec{
			ServiceAccountName: "riff-build",
			Source:             makeFunctionBuildSource(f),
			Template: &knbuildv1alpha1.TemplateInstantiationSpec{
				Name:      "riff-function",
				Kind:      "ClusterBuildTemplate",
				Arguments: makeFunctionBuildArguments(f),
			},
		},
	}

	return build
}

func makeFunctionBuildSource(f *buildv1alpha1.Function) *knbuildv1alpha1.SourceSpec {
	as := f.Spec.Source
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

func makeFunctionBuildArguments(f *buildv1alpha1.Function) []knbuildv1alpha1.ArgumentSpec {
	args := []knbuildv1alpha1.ArgumentSpec{
		{Name: "IMAGE", Value: f.Status.TargetImage},
		{Name: "FUNCTION_ARTIFACT", Value: f.Spec.Artifact},
		{Name: "FUNCTION_HANDLER", Value: f.Spec.Handler},
		{Name: "FUNCTION_LANGUAGE", Value: f.Spec.Invoker},
	}
	if f.Status.BuildCacheName != "" {
		args = append(args, knbuildv1alpha1.ArgumentSpec{Name: "CACHE", Value: "cache"})
	}
	return args
}

// MakeApplicationBuild creates a Build from an Application object.
func MakeApplicationBuild(a *buildv1alpha1.Application) *knbuildv1alpha1.Build {
	if a.Spec.Source == nil {
		return nil
	}

	build := &knbuildv1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ApplicationBuild(a),
			Namespace: a.Namespace,
			Labels:    makeApplicationLabels(a),
		},
		Spec: knbuildv1alpha1.BuildSpec{
			ServiceAccountName: "riff-build",
			Source:             makeApplicationBuildSource(a),
			Template: &knbuildv1alpha1.TemplateInstantiationSpec{
				Name:      "riff-application",
				Kind:      "ClusterBuildTemplate",
				Arguments: makeApplicationBuildArguments(a),
			},
		},
	}

	return build
}

func makeApplicationBuildSource(a *buildv1alpha1.Application) *knbuildv1alpha1.SourceSpec {
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

func makeApplicationBuildArguments(a *buildv1alpha1.Application) []knbuildv1alpha1.ArgumentSpec {
	args := []knbuildv1alpha1.ArgumentSpec{
		{Name: "IMAGE", Value: a.Status.TargetImage},
	}
	if a.Status.BuildCacheName != "" {
		args = append(args, knbuildv1alpha1.ArgumentSpec{Name: "CACHE", Value: "cache"})
	}
	return args
}
