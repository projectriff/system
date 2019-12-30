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

package build

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	buildv1alpha1 "github.com/projectriff/system/pkg/apis/build/v1alpha1"
	kpackbuildv1alpha1 "github.com/projectriff/system/pkg/apis/thirdparty/kpack/build/v1alpha1"
	"github.com/projectriff/system/pkg/controllers"
	"github.com/projectriff/system/pkg/refs"
)

const applicationIndexField = ".metadata.applicationController"

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=build.projectriff.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=build.projectriff.io,resources=applications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=build.pivotal.io,resources=images,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch

func (r *ApplicationReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("application", req.NamespacedName)

	var originalApplication buildv1alpha1.Application
	if err := r.Get(ctx, req.NamespacedName, &originalApplication); err != nil {
		if apierrs.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Application")
		return ctrl.Result{}, err
	}
	application := *(originalApplication.DeepCopy())

	application.Default()
	application.Status.InitializeConditions()

	result, err := r.reconcile(ctx, log, &application)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(application.Status, originalApplication.Status) && application.GetDeletionTimestamp() == nil {
		// update status
		log.Info("updating application status", "diff", cmp.Diff(originalApplication.Status, application.Status))
		if updateErr := r.Status().Update(ctx, &application); updateErr != nil {
			log.Error(updateErr, "unable to update Application status", "application", application)
			return ctrl.Result{Requeue: true}, updateErr
		}
	}

	// return original reconcile result
	return result, err
}

func (r *ApplicationReconciler) reconcile(ctx context.Context, log logr.Logger, application *buildv1alpha1.Application) (ctrl.Result, error) {
	if application.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	// resolve target image
	targetImage, err := r.resolveTargetImage(ctx, log, application)
	if err != nil {
		if err == errMissingDefaultPrefix {
			application.Status.MarkImageDefaultPrefixMissing(err.Error())
		} else {
			application.Status.MarkImageInvalid(err.Error())
		}
		return ctrl.Result{}, err
	}
	application.Status.MarkImageResolved()
	application.Status.TargetImage = targetImage

	// reconcile child kpack image
	childImage, err := r.reconcileChildKpackImage(ctx, log, application)
	if err != nil {
		log.Error(err, "unable to reconcile child Image", "application", application)
		return ctrl.Result{}, err
	}
	if childImage == nil {
		// TODO resolve to a digest
		application.Status.LatestImage = application.Status.TargetImage
		application.Status.MarkBuildNotUsed()
	} else {
		application.Status.KpackImageRef = refs.NewTypedLocalObjectReferenceForObject(childImage, r.Scheme)
		application.Status.LatestImage = childImage.Status.LatestImage
		application.Status.BuildCacheRef = refs.NewTypedLocalObjectReference(childImage.Status.BuildCacheName, schema.GroupKind{Kind: "PersistentVolumeClaim"})
		application.Status.PropagateKpackImageStatus(&childImage.Status)
	}

	application.Status.ObservedGeneration = application.Generation

	return ctrl.Result{}, nil
}

func (r *ApplicationReconciler) resolveTargetImage(ctx context.Context, log logr.Logger, application *buildv1alpha1.Application) (string, error) {
	if !strings.HasPrefix(application.Spec.Image, "_") {
		return application.Spec.Image, nil
	}

	var riffBuildConfig v1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Namespace: application.Namespace, Name: riffBuildServiceAccount}, &riffBuildConfig); err != nil {
		if apierrs.IsNotFound(err) {
			return "", errMissingDefaultPrefix
		}
		return "", err
	}
	defaultPrefix := riffBuildConfig.Data["default-image-prefix"]
	if defaultPrefix == "" {
		return "", errMissingDefaultPrefix
	}
	image, err := buildv1alpha1.ResolveDefaultImage(application, defaultPrefix)
	if err != nil {
		return "", err
	}
	return image, nil
}

func (r *ApplicationReconciler) reconcileChildKpackImage(ctx context.Context, log logr.Logger, application *buildv1alpha1.Application) (*kpackbuildv1alpha1.Image, error) {
	var actualImage kpackbuildv1alpha1.Image
	var childImages kpackbuildv1alpha1.ImageList
	if err := r.List(ctx, &childImages, client.InNamespace(application.Namespace), client.MatchingField(applicationIndexField, application.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childImages.Items) == 1 {
		actualImage = childImages.Items[0]
	} else if len(childImages.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraImage := range childImages.Items {
			log.Info("deleting extra kpack image", "image", extraImage)
			if err := r.Delete(ctx, &extraImage); err != nil {
				return nil, err
			}
		}
	}

	desiredImage, err := r.constructImageForApplication(application)
	if err != nil {
		return nil, err
	}

	if desiredImage == nil {
		// delete image if no longer needed
		if actualImage.Name != "" {
			log.Info("deleting kpack image", "image", actualImage)
			if err := r.Delete(ctx, &actualImage); err != nil {
				log.Error(err, "unable to delete kpack Image for Application", "image", actualImage)
				return nil, err
			}
		}
		return nil, nil
	}

	// create image if it doesn't exist
	if actualImage.Name == "" {
		log.Info("creating kpack image", "spec", desiredImage.Spec)
		if err := r.Create(ctx, desiredImage); err != nil {
			log.Error(err, "unable to create kpack Image for Application", "image", desiredImage)
			return nil, err
		}
		return desiredImage, nil
	}

	if r.imageSemanticEquals(desiredImage, &actualImage) {
		// image is unchanged
		return &actualImage, nil
	}

	// update image with desired changes
	image := actualImage.DeepCopy()
	image.ObjectMeta.Labels = desiredImage.ObjectMeta.Labels
	image.Spec = desiredImage.Spec
	log.Info("reconciling kpack image", "diff", cmp.Diff(actualImage.Spec, image.Spec))
	if err := r.Update(ctx, image); err != nil {
		log.Error(err, "unable to update kpack Image for Application", "image", image)
		return nil, err
	}

	return image, nil
}

func (r *ApplicationReconciler) imageSemanticEquals(desiredImage, image *kpackbuildv1alpha1.Image) bool {
	return equality.Semantic.DeepEqual(desiredImage.Spec, image.Spec) &&
		equality.Semantic.DeepEqual(desiredImage.ObjectMeta.Labels, image.ObjectMeta.Labels)
}

func (r *ApplicationReconciler) constructImageForApplication(application *buildv1alpha1.Application) (*kpackbuildv1alpha1.Image, error) {
	if application.Spec.Source == nil {
		return nil, nil
	}

	image := &kpackbuildv1alpha1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       r.constructLabelsForApplication(application),
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-application-", application.Name),
			Namespace:    application.Namespace,
		},
		Spec: kpackbuildv1alpha1.ImageSpec{
			Tag: application.Status.TargetImage,
			Builder: kpackbuildv1alpha1.ImageBuilder{
				TypeMeta: metav1.TypeMeta{
					Kind: "ClusterBuilder",
				},
				Name: "riff-application",
			},
			ServiceAccount:           riffBuildServiceAccount,
			Source:                   *application.Spec.Source,
			CacheSize:                application.Spec.CacheSize,
			FailedBuildHistoryLimit:  application.Spec.FailedBuildHistoryLimit,
			SuccessBuildHistoryLimit: application.Spec.SuccessBuildHistoryLimit,
			ImageTaggingStrategy:     application.Spec.ImageTaggingStrategy,
			Build:                    application.Spec.Build,
		},
	}
	if err := ctrl.SetControllerReference(application, image, r.Scheme); err != nil {
		return nil, err
	}

	return image, nil
}

func (r *ApplicationReconciler) constructLabelsForApplication(application *buildv1alpha1.Application) map[string]string {
	labels := make(map[string]string, len(application.ObjectMeta.Labels)+1)
	// pass through existing labels
	for k, v := range application.ObjectMeta.Labels {
		labels[k] = v
	}

	labels[buildv1alpha1.ApplicationLabelKey] = application.Name

	return labels
}

func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := controllers.IndexControllersOfType(mgr, applicationIndexField, &buildv1alpha1.Application{}, &kpackbuildv1alpha1.Image{}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&buildv1alpha1.Application{}).
		Owns(&kpackbuildv1alpha1.Image{}).
		Complete(r)
}
