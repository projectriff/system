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
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// CreateFunc is the signature of the client Create method.
type CreateFunc func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error

// CreateHook wraps the given create function. The hook could, for example, return an error or delegate to the create function.
type CreateHook func(createFake CreateFunc, ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error

// GetFunc is the signature of the client Get method.
type GetFunc func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error

// GetHook wraps the given get function. The hook could, for example, return an error or delegate to the get function.
// An alternative to using a GetHook to return "not found" is to omit the object from the initial set of objects.
type GetHook func(getFake GetFunc, ctx context.Context, key client.ObjectKey, obj runtime.Object) error

type clientWrapper struct {
	client        client.Client
	created       []runtime.Object
	statusUpdated []runtime.Object
	createHook    CreateHook
	getHook       GetHook
	genCount      int
}

var _ client.Client = &clientWrapper{}

func newClientWrapperWithScheme(clientScheme *runtime.Scheme, initObjs ...runtime.Object) *clientWrapper {
	return &clientWrapper{
		client:  fakeclient.NewFakeClientWithScheme(clientScheme, initObjs...),
		created: []runtime.Object{},
	}
}

func (w *clientWrapper) Create(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
	w.created = append(w.created, obj.DeepCopyObject())
	if accessor, ok := obj.(metav1.ObjectMetaAccessor); ok {
		objmeta := accessor.GetObjectMeta()
		if objmeta.GetName() == "" && objmeta.GetGenerateName() != "" {
			w.genCount++
			objmeta.SetName(fmt.Sprintf("%s%03d", objmeta.GetGenerateName(), w.genCount))
		}
	}
	if w.createHook != nil {
		return w.createHook(w.client.Create, ctx, obj, opts...)
	}
	return w.client.Create(ctx, obj, opts...)
}

func (w *clientWrapper) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	if w.getHook != nil {
		return w.getHook(w.client.Get, ctx, key, obj)
	}
	return w.client.Get(ctx, key, obj)
}

func (w *clientWrapper) List(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
	// TODO add a hook?
	return w.client.List(ctx, list, opts...)
}

func (w clientWrapper) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
	// TODO add a hook?
	return w.client.Delete(ctx, obj, opts...)
}

func (w clientWrapper) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	// TODO add a hook?
	return w.client.Update(ctx, obj, opts...)
}
func (w clientWrapper) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	// TODO add a hook?
	return w.client.Patch(ctx, obj, patch, opts...)
}

func (w clientWrapper) DeleteAllOf(ctx context.Context, obj runtime.Object, opts ...client.DeleteAllOfOption) error {
	// TODO add a hook?
	return w.client.DeleteAllOf(ctx, obj, opts...)
}

func (w *clientWrapper) Status() client.StatusWriter {
	return &statusWriterWrapper{
		statusWriter:  w.client.Status(),
		clientWrapper: w,
	}
}

type statusWriterWrapper struct {
	statusWriter  client.StatusWriter
	clientWrapper *clientWrapper
}

var _ client.StatusWriter = &statusWriterWrapper{}

func (w statusWriterWrapper) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	w.clientWrapper.statusUpdated = append(w.clientWrapper.statusUpdated, obj.DeepCopyObject())
	return w.statusWriter.Update(ctx, obj, opts...)
}

func (w statusWriterWrapper) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	panic("implement me")
}
