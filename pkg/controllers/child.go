/*
Copyright 2020 the original author or authors.

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

package controllers

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/projectriff/system/pkg/apis"
	"github.com/projectriff/system/pkg/tracker"
)

type ChildReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Tracker  tracker.Tracker

	ParentType    reflect.Type
	ChildType     reflect.Type
	ChildListType reflect.Type

	// IndexField is used to index objects of the child's type based on their
	// controlling owner. This field needs to be unique within the manager.
	IndexField string

	// SemanticEquals compares two child resources returning true if there is a
	// meaningful difference that should trigger an update.
	//
	// Expected function signature:
	//     func(a1, a2 apis.Object) bool
	SemanticEquals interface{}

	// ConstructDesiredChild returns the desired child object for the given
	// parent object, or nil if the child should not exist.
	//
	// Expected function signature:
	//     func(parent apis.Object) (apis.Object, error)
	ConstructDesiredChild interface{}

	// ReflectChildStatusOnParent updates the parent object's status with values
	// from the child. Select types of error are passed, including:
	// - apierrs.IsConflict
	//
	// Expected function signature:
	//     func(parent, child apis.Object, err error)
	ReflectChildStatusOnParent interface{}

	// HarmonizeImmutableFields allows fields that are immutable on the current
	// object to be copied to the desired object in order to avoid creating
	// updates which are guaranteed to fail.
	//
	// Expected function signature:
	//     func(current, desired apis.Object)
	//
	// +optional
	HarmonizeImmutableFields interface{}

	// MergeBeforeUpdate copies desired fields on to the current object before
	// calling update. Typically fields to copy are the Spec, Labels and
	// Annotations.
	//
	// Expected function signature:
	//     func(current, desired apis.Object)
	MergeBeforeUpdate interface{}

	// Sanitize is called with an object before logging the value. Any value may
	// be returned. A meaningful subset of the resource is typically returned,
	// like the Spec.
	//
	// Expected function signature:
	//     func(child apis.Object) interface{}
	//
	// +optional
	Sanitize interface{}
}

func (r *ChildReconciler) Reconcile(ctx context.Context, parent apis.Object) (ctrl.Result, error) {
	child, err := r.reconcile(ctx, parent)
	if err != nil {
		if apierrs.IsAlreadyExists(err) {
			r.Log.Info("unable to reconcile child, not owned", r.ParentType.Name(), parent, r.ChildType.Name(), r.sanitize(child))
			r.reflectChildStatusOnParent(parent, child, err)
			return ctrl.Result{}, nil
		}
		r.Log.Error(err, "unable to reconcile child", r.ParentType.Name(), parent)
		return ctrl.Result{}, err
	}
	r.reflectChildStatusOnParent(parent, child, err)

	return ctrl.Result{}, nil
}

func (r *ChildReconciler) reconcile(ctx context.Context, parent apis.Object) (apis.Object, error) {
	actual := reflect.New(r.ChildType).Interface().(apis.Object)
	children := reflect.New(r.ChildListType).Interface().(runtime.Object)
	if err := r.List(ctx, children, client.InNamespace(parent.GetNamespace()), client.MatchingField(r.IndexField, parent.GetName())); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	items := r.items(children)
	if len(items) == 1 {
		actual = items[0]
	} else if len(items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extra := range items {
			r.Log.Info("deleting extra child", r.ChildType.Name(), r.sanitize(extra))
			if err := r.Delete(ctx, extra); err != nil {
				r.Recorder.Eventf(parent, corev1.EventTypeWarning, "DeleteFailed",
					"Failed to delete %s %q: %v", r.ChildType.Name(), extra.GetName(), err)
				return nil, err
			}
			r.Recorder.Eventf(parent, corev1.EventTypeNormal, "Deleted",
				"Deleted %s %q", r.ChildType.Name(), extra.GetName())
		}
	}

	desired, err := r.constructDesiredChild(parent)
	if err != nil {
		return nil, err
	}

	// delete child if no longer needed
	if desired == nil {
		if !actual.GetCreationTimestamp().Time.IsZero() {
			r.Log.Info("deleting unwanted child", r.ChildType.Name(), r.sanitize(actual))
			if err := r.Delete(ctx, actual); err != nil {
				r.Log.Error(err, "unable to delete unwanted child", r.ChildType.Name(), r.sanitize(actual))
				r.Recorder.Eventf(parent, corev1.EventTypeWarning, "DeleteFailed",
					"Failed to delete %s %q: %v", r.ChildType.Name(), actual.GetName(), err)
				return nil, err
			}
			r.Recorder.Eventf(parent, corev1.EventTypeNormal, "Deleted",
				"Deleted %s %q", r.ChildType.Name(), actual.GetName())
		}
		return nil, nil
	}

	// create child if it doesn't exist
	if actual.GetName() == "" {
		r.Log.Info("creating child", r.ChildType.Name(), r.sanitize(desired))
		if err := r.Create(ctx, desired); err != nil {
			r.Log.Error(err, "unable to create child", r.ChildType.Name(), r.sanitize(desired))
			r.Recorder.Eventf(parent, corev1.EventTypeWarning, "CreationFailed",
				"Failed to create %s %q: %v", r.ChildType.Name(), desired.GetName(), err)
			return nil, err
		}
		r.Recorder.Eventf(parent, corev1.EventTypeNormal, "Created",
			"Created %s %q", r.ChildType.Name(), desired.GetName())
		return desired, nil
	}

	// overwrite fields that should not be mutated
	r.harmonizeImmutableFields(actual, desired)

	if r.semanticEquals(desired, actual) {
		// child is unchanged
		return actual, nil
	}

	// update child with desired changes
	current := actual.DeepCopyObject().(apis.Object)
	r.mergeBeforeUpdate(current, desired)
	r.Log.Info("reconciling child", "diff", cmp.Diff(r.sanitize(actual), r.sanitize(current)))
	if err := r.Update(ctx, current); err != nil {
		r.Log.Error(err, "unable to update child", r.ChildType.Name(), r.sanitize(current))
		r.Recorder.Eventf(parent, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update %s %q: %v", r.ChildType.Name(), current.GetName(), err)
		return nil, err
	}
	r.Recorder.Eventf(parent, corev1.EventTypeNormal, "Updated",
		"Updated %s %q", r.ChildType.Name(), current.GetName())

	return current, nil
}

func (r *ChildReconciler) semanticEquals(a1, a2 apis.Object) bool {
	fn := reflect.ValueOf(r.SemanticEquals)
	out := fn.Call([]reflect.Value{
		reflect.ValueOf(a1),
		reflect.ValueOf(a2),
	})
	return out[0].Bool()
}

func (r *ChildReconciler) constructDesiredChild(parent apis.Object) (apis.Object, error) {
	fn := reflect.ValueOf(r.ConstructDesiredChild)
	out := fn.Call([]reflect.Value{
		reflect.ValueOf(parent),
	})
	var obj apis.Object
	if !out[0].IsNil() {
		obj = out[0].Interface().(apis.Object)
	}
	var err error
	if !out[1].IsNil() {
		err = out[1].Interface().(error)
	}
	return obj, err
}

func (r *ChildReconciler) reflectChildStatusOnParent(parent, child apis.Object, err error) {
	fn := reflect.ValueOf(r.ReflectChildStatusOnParent)
	args := []reflect.Value{
		reflect.ValueOf(parent),
		reflect.ValueOf(child),
		reflect.ValueOf(err),
	}
	if parent == nil {
		args[0] = reflect.New(fn.Type().In(0)).Elem()
	}
	if child == nil {
		args[1] = reflect.New(fn.Type().In(1)).Elem()
	}
	if err == nil {
		args[2] = reflect.New(fn.Type().In(2)).Elem()
	}
	fn.Call(args)
}

func (r *ChildReconciler) harmonizeImmutableFields(current, desired apis.Object) {
	if r.HarmonizeImmutableFields == nil {
		return
	}
	fn := reflect.ValueOf(r.HarmonizeImmutableFields)
	fn.Call([]reflect.Value{
		reflect.ValueOf(current),
		reflect.ValueOf(desired),
	})
}

func (r *ChildReconciler) mergeBeforeUpdate(current, desired apis.Object) {
	fn := reflect.ValueOf(r.MergeBeforeUpdate)
	fn.Call([]reflect.Value{
		reflect.ValueOf(current),
		reflect.ValueOf(desired),
	})
}

func (r *ChildReconciler) sanitize(child apis.Object) interface{} {
	if r.Sanitize == nil {
		return child
	}
	if child == nil {
		return nil
	}
	fn := reflect.ValueOf(r.Sanitize)
	out := fn.Call([]reflect.Value{
		reflect.ValueOf(child),
	})
	var sanitized interface{}
	if !out[0].IsNil() {
		sanitized = out[0].Interface()
	}
	return sanitized
}

func (r *ChildReconciler) items(children runtime.Object) []apis.Object {
	childrenValue := reflect.ValueOf(children).Elem()
	itemsValue := childrenValue.FieldByName("Items")
	items := make([]apis.Object, itemsValue.Len())
	for i := range items {
		items[i] = itemsValue.Index(i).Addr().Interface().(apis.Object)
	}
	return items
}
