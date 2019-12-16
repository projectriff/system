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

	// GivenObjects holds the kubernetes objects which are present at the onset of reconciliation
	GivenObjects []runtime.Object

	// ExpectTracks holds the ordered list of Track calls expected during reconciliation
	ExpectTracks []TrackRequest

	// ExpectCreates holds the ordered list of objects expected to be created during reconciliation
	ExpectCreates []runtime.Object

	// ExpectStatusUpdates holds the ordered list of objects whose status is updated during reconciliation.
	ExpectStatusUpdates []runtime.Object

	// ShouldErr is true if and only if reconciliation is expected to return an error
	ShouldErr bool

	// ExpectErrMessage is the error message expected if ShouldErr is true
	ExpectErrMessage string

	// Hooks, if provided, wrap the corresponding client method. They are typically used to inject errors. The object
	// will be recorded as created if and only if the hook calls the wrapped method.
	CreateHook CreateHook
	GetHook    GetHook
}

// Table represents a list of Testcase tests instances.
type Table []Testcase

// Test executes the test for a table row.
func (r *Testcase) Test(t *testing.T, scheme *runtime.Scheme, factory Factory) {
	t.Helper()
	if r.Skip {
		t.SkipNow()
	}
	clientWrapper := newClientWrapperWithScheme(scheme, r.GivenObjects...)
	clientWrapper.createHook = r.CreateHook
	clientWrapper.getHook = r.GetHook
	tracker := createTracker()
	log := TestLogger(t)
	c := factory(t, r, clientWrapper, tracker, log)
	// Run the Reconcile we're testing.
	if _, err := c.Reconcile(reconcile.Request{
		NamespacedName: r.Key,
	}); (err != nil) != r.ShouldErr {
		t.Errorf("Reconcile() error = %v, ExpectErr %v", err, r.ShouldErr)
	} else {
		if r.ShouldErr && r.ExpectErrMessage != "" {
			if diff := cmp.Diff(r.ExpectErrMessage, err.Error()); diff != "" {
				t.Errorf("Unexpected error message(-expected, +actual): %s", diff)
			}
		}
	}

	actualTrack := tracker.getTrackRequests()
	for i, exp := range r.ExpectTracks {
		if i >= len(actualTrack) {
			t.Errorf("Missing tracking request: %s", exp)
			continue
		}

		if diff := cmp.Diff(exp, actualTrack[i]); diff != "" {
			t.Errorf("Unexpected tracking request(-expected, +actual): %s", diff)
		}
	}
	if actual, exp := len(actualTrack), len(r.ExpectTracks); actual > exp {
		for _, extra := range actualTrack[exp:] {
			t.Errorf("Extra tracking request: %s", extra)
		}
	}

	for i, exp := range r.ExpectCreates {
		if i >= len(clientWrapper.created) {
			t.Errorf("Missing create: %#v", exp)
			continue
		}
		actual := clientWrapper.created[i]

		if diff := cmp.Diff(exp, actual, ignoreLastTransitionTime, safeDeployDiff, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("Unexpected create (-expected, +actual): %s", diff)
		}
	}
	if actual, expected := len(clientWrapper.created), len(r.ExpectCreates); actual > expected {
		for _, extra := range clientWrapper.created[expected:] {
			t.Errorf("Extra create: %#v", extra)
		}
	}

	for i, exp := range r.ExpectStatusUpdates {
		if i >= len(clientWrapper.statusUpdated) {
			t.Errorf("Missing status update: %#v", exp)
			continue
		}
		actual := clientWrapper.statusUpdated[i]

		if diff := cmp.Diff(exp, actual, statusSubresourceOnly, ignoreLastTransitionTime, safeDeployDiff, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("Unexpected status update (-expected, +actual): %s", diff)
		}
	}
	if actual, expected := len(clientWrapper.statusUpdated), len(r.ExpectStatusUpdates); actual > expected {
		for _, extra := range clientWrapper.statusUpdated[expected:] {
			t.Errorf("Extra status update: %#v", extra)
		}
	}
}

var (
	ignoreLastTransitionTime = cmp.FilterPath(func(p cmp.Path) bool {
		return strings.HasSuffix(p.String(), "LastTransitionTime.Inner.Time")
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
