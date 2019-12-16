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
	"time"

	"github.com/go-logr/logr/testing"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/projectriff/system/pkg/tracker"
)

type TrackRequest struct {
	Ref tracker.Key
	Obj types.NamespacedName
}

func CreateTrackRequest(group, kind, namespace, name, controllerNamespace, controllerName string) TrackRequest {
	return TrackRequest{
		Ref: tracker.Key{GroupKind: schema.GroupKind{Group: group, Kind: kind}, NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}},
		Obj: types.NamespacedName{Namespace: controllerNamespace, Name: controllerName},
	}
}

const maxDuration = time.Duration(1<<63 - 1)

func createTracker() *mockTracker {
	return &mockTracker{Tracker: tracker.New(maxDuration, testing.NullLogger{}), reqs: []TrackRequest{}}
}

type mockTracker struct {
	tracker.Tracker
	reqs []TrackRequest
}

var _ tracker.Tracker = &mockTracker{}

func (t *mockTracker) Track(ref tracker.Key, obj types.NamespacedName) {
	t.Tracker.Track(ref, obj)
	t.reqs = append(t.reqs, TrackRequest{Ref: ref, Obj: obj})
}

func (t *mockTracker) getTrackRequests() []TrackRequest {
	result := []TrackRequest{}
	for _, req := range t.reqs {
		result = append(result, req)
	}
	return result
}
