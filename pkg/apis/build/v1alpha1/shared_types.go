/*
 * Copyright 2019 The original author or authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1alpha1

type BuildArgument struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Source struct {
	Git     *GitSource `json:"git"`
	SubPath string     `json:"subPath,omitempty"`
}

type GitSource struct {
	Revision string `json:"revision"`
	URL      string `json:"url"`
}

type BuildStatus struct {
	BuildCacheName string `json:"buildCacheName,omitempty"`
	BuildName      string `json:"buildName,omitempty"`
	LatestImage    string `json:"latestImage,omitempty"`
}
