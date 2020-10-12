/*
Copyright 2019-2020 InfluxData.

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
	defaultTelegrafImage  = "docker.io/library/telegraf:1.14"
	defaultRequestsCPU    = "10m"
	defaultRequestsMemory = "10Mi"
	defaultLimitsCPU      = "200m"
	defaultLimitsMemory   = "200Mi"
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var telegrafClassesDirectory string
	var defaultTelegrafClass string
	var telegrafImage string
	var enableDefaultInternalPlugin bool
	var telegrafRequestsCPU string
	var telegrafRequestsMemory string
	var telegrafLimitsCPU string
	var telegrafLimitsMemory string
	var enableIstioInjection bool
	var istioOutputClass string
	var istioTelegrafImage string
	var requireAnnotationsForSecret bool

	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&enableDefaultInternalPlugin, "enable-default-internal-plugin", false,
		"Enable internal plugin in telegraf for all sidecar. If disabled, can be set explicitly via appropriate annotation")
	flag.BoolVar(&requireAnnotationsForSecret, "require-annotations-for-secret", false,
		"Require the annotations to be present when updating a secret")
	flag.StringVar(&telegrafClassesDirectory, "telegraf-classes-directory", "/config/classes", "The name of the directory in which the telegraf classes are configured")
	flag.StringVar(&defaultTelegrafClass, "telegraf-default-class", "default", "Default telegraf class to use")
	flag.StringVar(&telegrafImage, "telegraf-image", defaultTelegrafImage, "Telegraf image to inject")
	flag.StringVar(&telegrafRequestsCPU, "telegraf-requests-cpu", defaultRequestsCPU, "Default requests for CPU")
	flag.StringVar(&telegrafRequestsMemory, "telegraf-requests-memory", defaultRequestsMemory, "Default requests for memory")
	flag.StringVar(&telegrafLimitsCPU, "telegraf-limits-cpu", defaultLimitsCPU, "Default limits for CPU")
	flag.StringVar(&telegrafLimitsMemory, "telegraf-limits-memory", defaultLimitsMemory, "Default limits for memory")
	flag.BoolVar(&enableIstioInjection, "enable-istio-injection", false,
		"Enable injecting additional sidecar for monitoring istio sidecar container. If enabled, additional sidecar telegraf-istio will be added for pods with the Istio annotation enabled")
	flag.StringVar(&istioOutputClass, "istio-output-class", "istio", "Class to use for adding telegraf-istio sidecar to monitor its sidecar")
	flag.StringVar(&istioTelegrafImage, "istio-telegraf-image", "", "If specified, use a custom image for telegraf-istio sidecar")
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

	logger := setupLog.WithName("podInjector")

	classData := &classDataHandler{
		Logger:                   logger,
		TelegrafClassesDirectory: telegrafClassesDirectory,
	}

	err = classData.validateClassData()
	if err != nil {
		setupLog.Error(err, "class data validation failed")
		os.Exit(1)
	}

	sidecar := &sidecarHandler{
		ClassDataHandler:            classData,
		Logger:                      logger,
		TelegrafDefaultClass:        defaultTelegrafClass,
		TelegrafImage:               telegrafImage,
		EnableDefaultInternalPlugin: enableDefaultInternalPlugin,
		EnableIstioInjection:        enableIstioInjection,
		IstioOutputClass:            istioOutputClass,
		IstioTelegrafImage:          istioTelegrafImage,
		RequestsCPU:                 telegrafRequestsCPU,
		RequestsMemory:              telegrafRequestsMemory,
		LimitsCPU:                   telegrafLimitsCPU,
		LimitsMemory:                telegrafLimitsMemory,
	}

	err = sidecar.validateRequestsAndLimits()
	if err != nil {
		setupLog.Error(err, "default resources validation failed")
		os.Exit(1)
	}

	hookServer.Register("/mutate-v1-pod", &webhook.Admission{Handler: &podInjector{
		Logger:                      logger,
		SidecarHandler:              sidecar,
		ClassDataHandler:            classData,
		RequireAnnotationsForSecret: requireAnnotationsForSecret,
	}})

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
