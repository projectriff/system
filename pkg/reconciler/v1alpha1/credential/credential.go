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

package credential

import (
	"context"
	"fmt"
	"strings"

	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/kmp"
	"github.com/knative/pkg/logging"
	"github.com/projectriff/system/pkg/apis/build"
	buildinformers "github.com/projectriff/system/pkg/client/informers/externalversions/build/v1alpha1"
	buildlisters "github.com/projectriff/system/pkg/client/listers/build/v1alpha1"
	"github.com/projectriff/system/pkg/reconciler"
	"github.com/projectriff/system/pkg/reconciler/digest"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	coreinformers "k8s.io/client-go/informers/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	// ReconcilerName is the name of the reconciler
	ReconcilerName      = "Credentials"
	controllerAgentName = "credential-controller"

	riffBuildServiceAccount = "riff-build"
)

// Reconciler implements controller.Reconciler for Credential resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	secretLister         corelisters.SecretLister
	serviceAccountLister corelisters.ServiceAccountLister
	applicationLister    buildlisters.ApplicationLister
	functionLister       buildlisters.FunctionLister

	resolver digest.Resolver
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt reconciler.Options,
	secretInformer coreinformers.SecretInformer,
	serviceAccountInformer coreinformers.ServiceAccountInformer,
	applicationInformer buildinformers.ApplicationInformer,
	functionInformer buildinformers.FunctionInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:                 reconciler.NewBase(opt, controllerAgentName),
		secretLister:         secretInformer.Lister(),
		serviceAccountLister: serviceAccountInformer.Lister(),
		applicationLister:    applicationInformer.Lister(),
		functionLister:       functionInformer.Lister(),

		resolver: digest.NewDefaultResolver(opt),
	}
	impl := controller.NewImpl(c, c.Logger, ReconcilerName, reconciler.MustNewStatsReporter(ReconcilerName, c.Logger))

	c.Logger.Info("Setting up event handlers")
	secretInformer.Informer().AddEventHandler(reconciler.Handler(func(obj interface{}) {
		meta, err := meta.Accessor(obj)
		if err != nil {
			return
		}
		if _, ok := meta.GetLabels()[build.CredentialLabelKey]; !ok {
			return
		}
		impl.EnqueueKey(meta.GetNamespace() + "/" + riffBuildServiceAccount)
	}))
	serviceAccountInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			meta, err := meta.Accessor(obj)
			if err != nil {
				return false
			}
			return meta.GetName() == riffBuildServiceAccount
		},
		Handler: reconciler.Handler(impl.Enqueue),
	})

	// Watch for new builds. If the registry is unauthenticated, there may not be a
	// credential to trigger the creation of the riff-build service account, but it
	// still needs to exist for builds to run.
	applicationInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			meta, err := meta.Accessor(obj)
			if err != nil {
				return
			}
			impl.EnqueueKey(meta.GetNamespace() + "/" + riffBuildServiceAccount)
		},
	})
	functionInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			meta, err := meta.Accessor(obj)
			if err != nil {
				return
			}
			impl.EnqueueKey(meta.GetNamespace() + "/" + riffBuildServiceAccount)
		},
	})

	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the ServiceAccount resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil || name != "riff-build" {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}

	// Get the ServiceAccount resource with this namespace/name
	original, err := c.serviceAccountLister.ServiceAccounts(namespace).Get(name)
	if err != nil && !apierrs.IsNotFound(err) {
		return err
	}

	// Don't modify the informers copy
	var serviceAccount *corev1.ServiceAccount
	if original != nil {
		serviceAccount = original.DeepCopy()
	}

	// Reconcile this copy of the serviceAccount
	serviceAccount, err = c.reconcile(ctx, serviceAccount, namespace)

	if err == nil && serviceAccount != nil {
		// If there was a difference and there was no error.
		c.Recorder.Eventf(serviceAccount, corev1.EventTypeNormal, "Updated", "Reconciled ServiceAccount credentials %q", serviceAccount.GetName())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, serviceAccount *corev1.ServiceAccount, namespace string) (*corev1.ServiceAccount, error) {
	logger := logging.FromContext(ctx)
	if serviceAccount != nil && serviceAccount.GetDeletionTimestamp() != nil {
		return serviceAccount, nil
	}

	secretLabelSelector, _ := labels.Parse(build.CredentialLabelKey)
	secretNames := sets.NewString()
	if secrets, err := c.secretLister.Secrets(namespace).List(secretLabelSelector); err != nil {
		logger.Errorf("Failed to reconcile ServiceAccount: %q failed to Get Secrets; %v", serviceAccount.Name, zap.Error(err))
		return serviceAccount, err
	} else {
		for _, secret := range secrets {
			secretNames.Insert(secret.Name)
		}
	}
	serviceAccountName := riffBuildServiceAccount

	if serviceAccount == nil {
		if needed, err := c.isServiceAccountNeeded(secretNames, namespace); err != nil {
			return serviceAccount, err
		} else if needed {
			serviceAccount, err := c.createServiceAccount(secretNames, namespace)
			if err != nil {
				logger.Errorf("Failed to create ServiceAccount %q: %v", serviceAccountName, err)
				c.Recorder.Eventf(serviceAccount, corev1.EventTypeWarning, "CreationFailed", "Failed to create ServiceAccount %q: %v", serviceAccountName, err)
				return serviceAccount, err
			}
			if serviceAccount != nil {
				c.Recorder.Eventf(serviceAccount, corev1.EventTypeNormal, "Created", "Created ServiceAccount %q", serviceAccountName)
			}
		}
	} else {
		serviceAccount, err := c.reconcileServiceAccount(ctx, serviceAccount, secretNames)
		if err != nil {
			logger.Errorf("Failed to reconcile ServiceAccount: %q; %v", serviceAccount.Name, zap.Error(err))
			return serviceAccount, err
		}
	}

	return serviceAccount, nil
}

func (c *Reconciler) reconcileServiceAccount(ctx context.Context, existingServiceAccount *corev1.ServiceAccount, desiredBoundSecrets sets.String) (*corev1.ServiceAccount, error) {
	logger := logging.FromContext(ctx)

	serviceAccount := existingServiceAccount.DeepCopy()
	boundSecrets := sets.NewString(strings.Split(serviceAccount.Annotations[build.CredentialsAnnotationKey], ",")...)
	removeSecrets := boundSecrets.Difference(desiredBoundSecrets)

	secrets := []corev1.ObjectReference{}
	// filter out secrets no longer bound
	for _, secret := range serviceAccount.Secrets {
		if !removeSecrets.Has(secret.Name) {
			secrets = append(secrets, secret)
		}
	}
	// add new secrets
	for _, secret := range desiredBoundSecrets.Difference(boundSecrets).UnsortedList() {
		secrets = append(secrets, corev1.ObjectReference{Name: secret})
	}
	serviceAccount.Secrets = secrets

	if serviceAccount.Annotations == nil {
		serviceAccount.Annotations = map[string]string{}
	}
	serviceAccount.Annotations[build.CredentialsAnnotationKey] = strings.Join(desiredBoundSecrets.List(), ",")

	if serviceAccountSemanticEquals(serviceAccount, existingServiceAccount) {
		// No differences to reconcile.
		return serviceAccount, nil
	}
	diff, err := kmp.SafeDiff(serviceAccount.Secrets, existingServiceAccount.Secrets)
	if err != nil {
		return nil, fmt.Errorf("failed to diff ServiceAccount: %v", err)
	}
	logger.Infof("Reconciling build cache diff (-desired, +observed): %s", diff)

	return c.KubeClientSet.CoreV1().ServiceAccounts(serviceAccount.Namespace).Update(serviceAccount)
}

func (c *Reconciler) isServiceAccountNeeded(secretNames sets.String, namespace string) (bool, error) {
	if secretNames.Len() != 0 {
		return true, nil
	}
	if items, err := c.applicationLister.Applications(namespace).List(labels.Everything()); err != nil {
		return false, err
	} else if len(items) != 0 {
		return true, nil
	}
	if items, err := c.functionLister.Functions(namespace).List(labels.Everything()); err != nil {
		return false, err
	} else if len(items) != 0 {
		return true, nil
	}
	return false, nil
}

func (c *Reconciler) createServiceAccount(secretNames sets.String, namespace string) (*corev1.ServiceAccount, error) {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      riffBuildServiceAccount,
			Namespace: namespace,
			Annotations: map[string]string{
				build.CredentialsAnnotationKey: strings.Join(secretNames.List(), ","),
			},
		},
		Secrets: make([]corev1.ObjectReference, secretNames.Len()),
	}
	for i, secretName := range secretNames.UnsortedList() {
		serviceAccount.Secrets[i] = corev1.ObjectReference{Name: secretName}
	}
	return c.KubeClientSet.CoreV1().ServiceAccounts(namespace).Create(serviceAccount)
}

func serviceAccountSemanticEquals(desiredServiceAccount, serviceAccount *corev1.ServiceAccount) bool {
	return equality.Semantic.DeepEqual(desiredServiceAccount.Secrets, serviceAccount.Secrets) &&
		equality.Semantic.DeepEqual(desiredServiceAccount.Annotations, serviceAccount.Annotations)
}
