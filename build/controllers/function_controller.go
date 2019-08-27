/*
Copyright 2019 The original author or authors.

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

	"github.com/go-logr/logr"
	buildv1alpha1 "github.com/projectriff/system/api/v1alpha1"
	"github.com/projectriff/system/controllers/names"
	"github.com/projectriff/system/controllers/resources"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FunctionReconciler reconciles a Function object
type FunctionReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=build.projectriff.io,resources=functions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=build.projectriff.io,resources=functions/status,verbs=get;update;patch

func (r *FunctionReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("function", req.NamespacedName)

	var function buildv1alpha1.Function
	if err := r.Get(ctx, req.NamespacedName, &function); err != nil {
		log.Error(err, "unable to fetch function")
		// ignore notFound errors
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	buildCacheName := names.BuildCache(&function)
	var buildCache v1.PersistentVolumeClaim
	var err error
	if err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: buildCacheName}, &buildCache); err != nil {
		if errors.IsNotFound(err) {
			// create a new pvc
			buildCache, err := resources.MakeBuildCache(&function)
			if err != nil {
				return ctrl.Result{}, err
			}
			// TODO add owner reference
			err = r.Create(ctx, buildCache)
			if err != nil {
				return ctrl.Result{}, err
			}
			function.Status.BuildCacheName = buildCache.Name
		}

	}
	// update status
	if err = r.Status().Update(ctx, &function); err != nil {
		log.Error(err, "unable to update function status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *FunctionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&buildv1alpha1.Function{}).
		Complete(r)
}
