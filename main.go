/*
Copyright 2019 Influxdata.

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
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"

	// +kubebuilder:scaffold:imports

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

const (
	defaultTelegrafImage = "docker.io/library/telegraf:1.13"
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var telegrafClassesSecretName string
	var defaultTelegrafClass string
	var controllerNamespace string
	var telegrafImage string
	var enableDefaultInternalPlugin bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&enableDefaultInternalPlugin, "enable-default-internal-plugin", false,
		"Enable internal plugin in telegraf for all sidecar. If disabled, can be set explicitly via appropriate annotation")
	flag.StringVar(&telegrafClassesSecretName, "telegraf-classes-secret", "telegraf-classes", "The name of the secret in which are configured the telegraf classes")
	flag.StringVar(&defaultTelegrafClass, "telegraf-default-class", "default", "Default telegraf class to use")
	flag.StringVar(&controllerNamespace, "namespace", os.Getenv("POD_NAMESPACE"), "Namespace in whick this pod is running in")
	flag.StringVar(&telegrafImage, "telegraf-image", defaultTelegrafImage, "Telegraf image to inject")
	flag.Parse()

	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = true
	}))
	entryLog := setupLog.WithName("entrypoint")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		Port:               9443,
		CertDir:            "/etc/certs",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	// Setup webhooks
	entryLog.Info("setting up webhook server")
	hookServer := mgr.GetWebhookServer()

	entryLog.Info("registering webhooks to the webhook server")

	hookServer.Register("/mutate-v1-pod", &webhook.Admission{Handler: &podInjector{
		TelegrafClassesSecretName: telegrafClassesSecretName,
		TelegrafDefaultClass:      defaultTelegrafClass,
		ControllerNamespace:       controllerNamespace,
		Logger:                    setupLog.WithName("podInjector"),
		SidecarConfig: &sidecarConfig{
			TelegrafImage:               telegrafImage,
			EnableDefaultInternalPlugin: enableDefaultInternalPlugin,
		},
	}})

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
