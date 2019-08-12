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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/projectriff/system/pkg/apis/core"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDeployment(t *testing.T) {
	d := createDeployerMeta()
	d.Labels = map[string]string{testLabelKey: testLabelValue}
	d.Spec.Template.ServiceAccountName = testServiceAccount
	d.Spec.Template.Containers[0].Image = testImage

	kd, _ := MakeDeployment(d)

	if got, want := kd.Name, testDeploymentName; got != want {
		t.Errorf("expected %q for deployment name got %q", want, got)
	}
	if got, want := kd.Namespace, testDeployerNamespace; got != want {
		t.Errorf("expected %q for deployment namespace got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, kd.OwnerReferences)

	if got, want := len(kd.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := kd.Labels[testLabelKey], testLabelValue; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := kd.Labels[core.DeployerLabelKey], testDeployerName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if diff := cmp.Diff(kd.Labels, kd.Spec.Template.Labels); diff != "" {
		t.Errorf("deployment and pod template labels differ (-want, +got) = %v", diff)
	}

	podSpec := kd.Spec.Template.Spec.DeepCopy()
	podSpec.Containers[0].LivenessProbe = nil
	podSpec.Containers[0].ReadinessProbe = nil
	if diff := cmp.Diff(d.Spec.Template, podSpec); diff != "" {
		t.Errorf("deployment and deployer pod templates differ (-want, +got) = %v", diff)
	}
	defaultProbe := &corev1.Probe{
		Handler: corev1.Handler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt(8080),
			},
		},
	}
	if diff := cmp.Diff(defaultProbe, kd.Spec.Template.Spec.Containers[0].LivenessProbe); diff != "" {
		t.Errorf("deployment and deployer liveness probes differ (-want, +got) = %v", diff)
	}
	if diff := cmp.Diff(defaultProbe, kd.Spec.Template.Spec.Containers[0].ReadinessProbe); diff != "" {
		t.Errorf("deployment and deployer readiness probes differ (-want, +got) = %v", diff)
	}
}
