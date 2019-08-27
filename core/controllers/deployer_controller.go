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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/projectriff/system/api/v1alpha1"
)

// DeployerReconciler reconciles a Deployer object
type DeployerReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=core.projectriff.io,resources=deployers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.projectriff.io,resources=deployers/status,verbs=get;update;patch

func (r *DeployerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("deployer", req.NamespacedName)

	var deployer corev1alpha1.Deployer
	if err := r.Get(ctx, req.NamespacedName, &deployer); err != nil {
		log.Error(err, "unable to fetch core deployer")
		// ignore notFound errors
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	build := deployer.Spec.Build
	if build == nil {
		// nothing to do
		return ctrl.Result{}, nil
	}
	switch {
	case build.ApplicationRef != "":

	}

	return ctrl.Result{}, nil
}

func (r *DeployerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.Deployer{}).
		Complete(r)
}
