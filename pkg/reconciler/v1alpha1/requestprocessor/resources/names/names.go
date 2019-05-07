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

package names

import (
	"fmt"

	requestv1alpha1 "github.com/projectriff/system/pkg/apis/request/v1alpha1"
)

func Items(rp *requestv1alpha1.RequestProcessor) []string {
	names := make([]string, len(rp.Spec))
	for i := range rp.Spec {
		names[i] = Item(rp, i)
	}
	return names
}

func Item(rp *requestv1alpha1.RequestProcessor, i int) string {
	return fmt.Sprintf("%s-%s", rp.Name, rp.Spec[i].Name)
}

func Route(rp *requestv1alpha1.RequestProcessor) string {
	return rp.Name
}
