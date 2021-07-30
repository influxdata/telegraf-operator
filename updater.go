package main

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// secretsUpdater updates all secrets managed by telegraf-operator whose contents have changed in all namespaces.
type secretsUpdater struct {
	logger       logr.Logger
	clientset    kubernetes.Interface
	batchDelay   time.Duration
	assembleConf func(*corev1.Pod, string) (string, error)
}

// newSecretsUpdater creates new instance of secretsUpdater.
func newSecretsUpdater(logger logr.Logger, sidecar *sidecarHandler) (*secretsUpdater, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &secretsUpdater{
		logger:       logger,
		clientset:    clientset,
		batchDelay:   10 * time.Second,
		assembleConf: sidecar.assembleConf,
	}, nil
}

// onChange updates secrets all namespaces, handling and logging errors internally
func (u *secretsUpdater) onChange() {
	u.logger.Info("checking secrets for updater")

	ctx := context.Background()

	// find all namespaces and find all telegraf-operator managed secrets in each namespace
	namespaces, err := u.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		u.logger.Error(err, "unable to list namespaces")
		return
	}

	// iterate over all namespaces, trying to update all telegraf-operator managed secrets
	for _, namespace := range namespaces.Items {
		err = u.updateSecretsInNamespace(ctx, namespace.Name)
		if err != nil {
			u.logger.Error(err, "unable to update secrets", "namespace", namespace)
			return
		}
	}
}

// updateSecretsInNamespace updates secrets in a single namespace, returning errors if they occur
func (u *secretsUpdater) updateSecretsInNamespace(ctx context.Context, namespace string) error {
	secretsClient := u.clientset.CoreV1().Secrets(namespace)

	// find all secrets having the label set by telegraf-operator, limiting results only to secrets
	// that the telegraf-operator is managing
	secrets, err := secretsClient.List(ctx, metav1.ListOptions{
		LabelSelector: TelegrafSecretLabelClassName,
	})
	if err != nil {
		return err
	}

	for _, secret := range secrets.Items {
		// get the pod and class name labels
		podName := secret.GetLabels()[TelegrafSecretLabelPod]
		className := secret.GetLabels()[TelegrafSecretLabelClassName]

		// if one of the labels was not present, throw an error
		if podName == "" || className == "" {
			return fmt.Errorf(`unable to get pod and class name for secret %s in namespace %s; podName="%s"; className="%s"`, secret.Name, secret.Namespace, podName, className)
		}

		// get the pod that the secret is used in
		pod, err := u.clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return err
		}

		telegrafConf, err := u.assembleConf(pod, className)
		if err != nil {
			return err
		}

		// check whether secret should be updated, perform the update if needed
		if string(secret.Data[TelegrafSecretDataKey]) != telegrafConf {
			u.logger.Info("updating secret", "namespace", namespace, "name", secret.Name, "podName", podName, "class", className)
			secret.Data[TelegrafSecretDataKey] = []byte(telegrafConf)

			_, err = secretsClient.Update(ctx, &secret, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		} else {
			u.logger.Info("not updating secret", "namespace", namespace, "name", secret.Name, "podName", podName, "class", className)
		}
	}

	return nil
}
