/*
Copyright 2019 the original author or authors.

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

package testing

import (
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/projectriff/system/pkg/tracker"
)

// Testcase holds a single row of a table test.
type Testcase struct {
	// Name is a descriptive name for this test suitable as a first argument to t.Run()
	Name string
	// Focus is true if and only if only this and any other focussed tests are to be executed.
	// If one or more tests are focussed, the overall table test will fail.
	Focus bool
	// Skip is true if and only if this test should be skipped.
	Skip bool

	// Key identifies the object to be reconciled
	Key types.NamespacedName

	// WithReactors installs each ReactionFunc into each fake clientset. ReactionFuncs intercept
	// each call to the clientset providing the ability to mutate the resource or inject an error.
	WithReactors []ReactionFunc
	// GivenObjects holds the kubernetes objects which are present at the onset of reconciliation
	GivenObjects []runtime.Object

	// ExpectTracks holds the ordered list of Track calls expected during reconciliation
	ExpectTracks []TrackRequest
	// ExpectCreates holds the ordered list of objects expected to be created during reconciliation
	ExpectCreates []runtime.Object
	// ExpectUpdates holds the ordered list of objects expected to be updated during reconciliation
	ExpectUpdates []runtime.Object
	// ExpectDeletes holds the ordered list of objects expected to be deleted during reconciliation
	ExpectDeletes []DeleteRef
	// ExpectStatusUpdates holds the ordered list of objects whose status is updated during reconciliation
	ExpectStatusUpdates []runtime.Object

	// ShouldErr is true if and only if reconciliation is expected to return an error
	ShouldErr bool
	// ExpectedResult is compared to the result returned from the reconciler if there was no error
	ExpectedResult controllerruntime.Result
	// Verify provides the reconciliation Result and error for custom assertions
	Verify VerifyFunc
}

// VerifyFunc is a verification function
type VerifyFunc func(t *testing.T, result controllerruntime.Result, err error)

// Table represents a list of Testcase tests instances.
type Table []Testcase

// Test executes the test for a table row.
func (tc *Testcase) Test(t *testing.T, scheme *runtime.Scheme, factory Factory) {
	t.Helper()
	if tc.Skip {
		t.SkipNow()
	}
	clientWrapper := newClientWrapperWithScheme(scheme, tc.GivenObjects...)
	for i := range tc.WithReactors {
		// in reverse order since we prepend
		reactor := tc.WithReactors[len(tc.WithReactors)-1-i]
		clientWrapper.PrependReactor("*", "*", reactor)
	}
	tracker := createTracker()
	log := TestLogger(t)
	c := factory(t, tc, clientWrapper, tracker, log)

	// Run the Reconcile we're testing.
	result, err := c.Reconcile(reconcile.Request{
		NamespacedName: tc.Key,
	})

	if (err != nil) != tc.ShouldErr {
		t.Errorf("Reconcile() error = %v, ExpectErr %v", err, tc.ShouldErr)
	}
	if err == nil {
		// result is only significant if there wasn't an error
		if diff := cmp.Diff(tc.ExpectedResult, result); diff != "" {
			t.Errorf("Unexpected result (-expected, +actual): %s", diff)
		}
	}

	if tc.Verify != nil {
		tc.Verify(t, result, err)
	}

	actualTrack := tracker.getTrackRequests()
	for i, exp := range tc.ExpectTracks {
		if i >= len(actualTrack) {
			t.Errorf("Missing tracking request: %s", exp)
			continue
		}

		if diff := cmp.Diff(exp, actualTrack[i]); diff != "" {
			t.Errorf("Unexpected tracking request(-expected, +actual): %s", diff)
		}
	}
	if actual, exp := len(actualTrack), len(tc.ExpectTracks); actual > exp {
		for _, extra := range actualTrack[exp:] {
			t.Errorf("Extra tracking request: %s", extra)
		}
	}

	for i, exp := range tc.ExpectCreates {
		if i >= len(clientWrapper.createActions) {
			t.Errorf("Missing create: %#v", exp)
			continue
		}
		actual := clientWrapper.createActions[i].GetObject()

		if diff := cmp.Diff(exp, actual, ignoreLastTransitionTime, safeDeployDiff, ignoreTypeMeta, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("Unexpected create (-expected, +actual): %s", diff)
		}
	}
	if actual, expected := len(clientWrapper.createActions), len(tc.ExpectCreates); actual > expected {
		for _, extra := range clientWrapper.createActions[expected:] {
			t.Errorf("Extra create: %#v", extra)
		}
	}

	for i, exp := range tc.ExpectUpdates {
		if i >= len(clientWrapper.updateActions) {
			t.Errorf("Missing update: %#v", exp)
			continue
		}
		actual := clientWrapper.updateActions[i].GetObject()

		if diff := cmp.Diff(exp, actual, ignoreLastTransitionTime, safeDeployDiff, ignoreTypeMeta, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("Unexpected update (-expected, +actual): %s", diff)
		}
	}
	if actual, expected := len(clientWrapper.updateActions), len(tc.ExpectUpdates); actual > expected {
		for _, extra := range clientWrapper.updateActions[expected:] {
			t.Errorf("Extra update: %#v", extra)
		}
	}

	for i, exp := range tc.ExpectDeletes {
		if i >= len(clientWrapper.deleteActions) {
			t.Errorf("Missing delete: %#v", exp)
			continue
		}
		actual := NewDeleteRef(clientWrapper.deleteActions[i])

		if diff := cmp.Diff(exp, actual); diff != "" {
			t.Errorf("Unexpected delete (-expected, +actual): %s", diff)
		}
	}
	if actual, expected := len(clientWrapper.deleteActions), len(tc.ExpectDeletes); actual > expected {
		for _, extra := range clientWrapper.deleteActions[expected:] {
			t.Errorf("Extra delete: %#v", extra)
		}
	}

	for i, exp := range tc.ExpectStatusUpdates {
		if i >= len(clientWrapper.statusUpdateActions) {
			t.Errorf("Missing status update: %#v", exp)
			continue
		}
		actual := clientWrapper.statusUpdateActions[i].GetObject()

		if diff := cmp.Diff(exp, actual, statusSubresourceOnly, ignoreLastTransitionTime, safeDeployDiff, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("Unexpected status update (-expected, +actual): %s", diff)
		}
	}
	if actual, expected := len(clientWrapper.statusUpdateActions), len(tc.ExpectStatusUpdates); actual > expected {
		for _, extra := range clientWrapper.statusUpdateActions[expected:] {
			t.Errorf("Extra status update: %#v", extra)
		}
	}
}

var (
	ignoreLastTransitionTime = cmp.FilterPath(func(p cmp.Path) bool {
		return strings.HasSuffix(p.String(), "LastTransitionTime.Inner.Time")
	}, cmp.Ignore())
	ignoreTypeMeta = cmp.FilterPath(func(p cmp.Path) bool {
		return strings.HasSuffix(p.String(), "TypeMeta.APIVersion") || strings.HasSuffix(p.String(), "TypeMeta.Kind")
	}, cmp.Ignore())

	statusSubresourceOnly = cmp.FilterPath(func(p cmp.Path) bool {
		q := p.String()
		return q != "" && !strings.HasPrefix(q, "Status")
	}, cmp.Ignore())

	safeDeployDiff = cmpopts.IgnoreUnexported(resource.Quantity{})
)

// Test executes the whole suite of the table tests.
func (tb Table) Test(t *testing.T, scheme *runtime.Scheme, factory Factory) {
	t.Helper()
	focussed := Table{}
	for _, test := range tb {
		if test.Focus {
			focussed = append(focussed, test)
			break
		}
	}
	testsToExecute := tb
	if len(focussed) > 0 {
		testsToExecute = focussed
	}
	for _, test := range testsToExecute {
		// Record the given objects
		givenObjects := make([]runtime.Object, 0, len(test.GivenObjects))
		for _, obj := range test.GivenObjects {
			givenObjects = append(givenObjects, obj.DeepCopyObject())
		}

		t.Run(test.Name, func(t *testing.T) {
			t.Helper()
			test.Test(t, scheme, factory)
		})

		// Validate the given objects are not mutated by reconciliation
		if diff := cmp.Diff(givenObjects, test.GivenObjects, safeDeployDiff, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("Given objects mutated by test %s (-expected, +actual): %v", test.Name, diff)
		}
	}
	if len(focussed) > 0 {
		t.Errorf("%d tests out of %d are still focussed, so the table test fails", len(focussed), len(tb))
	}
}

// Factory returns a Reconciler.Interface to perform reconciliation in table test,
// ActionRecorderList/EventList to capture k8s actions/events produced during reconciliation
// and FakeStatsReporter to capture stats.
type Factory func(t *testing.T, row *Testcase, client client.Client, tracker tracker.Tracker, log logr.Logger) reconcile.Reconciler

type DeleteRef struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
}

func NewDeleteRef(action DeleteAction) DeleteRef {
	return DeleteRef{
		Group:     action.GetResource().Group,
		Kind:      action.GetResource().Resource,
		Namespace: action.GetNamespace(),
		Name:      action.GetName(),
	}
}
