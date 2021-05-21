/*
Copyright 2018 The Kubernetes Authors.
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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/go-logr/logr"
	admv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiserver/pkg/storage/names"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,failurePolicy=fail,groups="",resources=pods,verbs=create;update,versions=v1,name=mpod.kb.io,sideEffects=None,admissionReviewVersions=v1

// podInjector inject telegraf Pods
type podInjector struct {
	client  client.Client
	decoder *admission.Decoder
	names.NameGenerator
	Logger                      logr.Logger
	ClassDataHandler            *classDataHandler
	SidecarHandler              *sidecarHandler
	RequireAnnotationsForSecret bool
}

// podInjector adds an annotation to every incoming pods.
func (a *podInjector) Handle(ctx context.Context, req admission.Request) admission.Response {
	handlerLog := setupLog.WithName("inject-handler")

	marshaled, err := json.Marshal(req)
	if err != nil {
		log.Fatal(err)
	}
	handlerLog.V(9).Info("request=" + string(marshaled))

	if req.Operation == admv1.Delete {
		deleteFailed := false
		for _, name := range a.SidecarHandler.telegrafSecretNames(req.Name) {
			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: req.Namespace,
				},
			}
			handlerLog.Info("Deleting secret=" + secret.Name + "/" + secret.Namespace)
			err := a.client.Delete(ctx, secret)
			if err != nil {
				handlerLog.Info("secret=" + secret.Name + "/" + secret.Namespace + " error:" + err.Error())
				deleteFailed = true
			}
		}
		if deleteFailed {
			return admission.Allowed("telegraf-injector couldn't delete one or more secrets")
		}

		return admission.Allowed("telegraf-injector doesn't block pod deletions")
	}

	pod := &corev1.Pod{}
	err = a.decoder.Decode(req, pod)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if a.SidecarHandler.skip(pod) {
		a.Logger.Info("skipping pod as telegraf-injector should not handle it")
		return admission.Allowed("telegraf-injector has no power over this pod")
	}

	name := pod.GetName()
	if name == "" {
		name = names.SimpleNameGenerator.GenerateName(pod.GetGenerateName())
		pod.SetName(name)
		handlerLog.Info("name: " + name + ",  pod_getname=" + pod.GetName())
	}

	a.Logger.Info("adding sidecar container")
	// if the telegraf configuration could be created, add sidecar pod
	result, err := a.SidecarHandler.addSidecars(pod, pod.GetName(), req.Namespace)
	if err != nil {

		if nonFatalErr, ok := err.(*nonFatalError); ok {
			a.Logger.Info(
				fmt.Sprintf(
					"unable to add telegraf sidecar container(s): %v ; not adding sidecar container, but allowing creation: %s",
					nonFatalErr.err,
					nonFatalErr.message,
				),
			)
			return admission.Allowed(nonFatalErr.message)
		}

		a.Logger.Info(fmt.Sprintf("unable to add telegraf sidecar container(s): %v ; reporting error", err))
		return admission.Errored(http.StatusBadRequest, err)
	}

	if req.Operation == admv1.Create || req.Operation == admv1.Update {
		err = a.createOrUpdateSecrets(ctx, result.secrets)
		if err != nil {
			a.Logger.Error(err, "unable to create secret")
			return admission.Errored(http.StatusBadRequest, err)
		}
	}

	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		a.Logger.Error(err, "unable to marshal JSON")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

// podInjector implements inject.Client.
// A client will be automatically injected.

// InjectClient injects the client.
func (a *podInjector) InjectClient(c client.Client) error {
	a.client = c

	return nil
}

// podInjector implements admission.DecoderInjector.
// A decoder will be automatically injected.

// InjectDecoder injects the decoder.
func (a *podInjector) InjectDecoder(d *admission.Decoder) error {
	a.decoder = d
	return nil
}

// createOrUpdateSecrets creates or updates all of the required secrets
func (a *podInjector) createOrUpdateSecrets(ctx context.Context, secrets []*corev1.Secret) error {
	for _, secret := range secrets {
		err := a.client.Create(ctx, secret)
		if errors.IsAlreadyExists(err) {
			existingSecret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
			}
			namespacedName := types.NamespacedName{
				Name:      secret.Name,
				Namespace: secret.Namespace,
			}

			err = a.client.Get(ctx, namespacedName, existingSecret)
			if err != nil {
				a.Logger.Error(err, fmt.Sprintf("unable to get secret %s in namespace %s", secret.Name, secret.Namespace))
				return err
			}

			if !a.isSecretManagedByTelegrafOperator(existingSecret) {
				err = fmt.Errorf("unable to update existing secret %s in namespace %s as it is not managed by telegraf-operator", secret.Name, secret.Namespace)
				return err
			}

			err = a.client.Update(ctx, secret)
			if err != nil {
				a.Logger.Error(err, fmt.Sprintf("unable to update secret %s in namespace %s", secret.Name, secret.Namespace))
				return err
			}
		} else if err != nil {
			a.Logger.Error(err, fmt.Sprintf("unable to create secret %s in namespace %s", secret.Name, secret.Namespace))
			return err
		}
	}

	return nil
}

func (a *podInjector) isSecretManagedByTelegrafOperator(secret *corev1.Secret) bool {
	// verify the secret is of type Opaque
	if secret.Type != "Opaque" {
		a.Logger.Info("assuming secret already exists and is not telegraf-matched as its type is not Opaque")
		return false
	}
	// verify that the secret only contains the expected key
	if len(secret.Data) != 1 || len(secret.Data[TelegrafSecretDataKey]) == 0 {
		a.Logger.Info("assuming secret already exists and is not telegraf-matched as its data has non-standard keys")
		return false
	}
	if a.RequireAnnotationsForSecret && !(secret.GetAnnotations()[TelegrafSecretAnnotationKey] == TelegrafSecretAnnotationValue) {
		a.Logger.Info("assuming secret already exists and is not telegraf-matched as it is missing the annotation")
		return false
	}

	return true
}
