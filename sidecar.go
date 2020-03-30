package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/influxdata/toml"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// TelegrafAnnotationCommon is the shared prefix for all annotations.
	TelegrafAnnotationCommon = "telegraf.influxdata.com"
	// TelegrafMetricsPort is used to configure a port telegraf should scrape;
	// Equivalent to TelegrafMetricsPorts: "6060"
	TelegrafMetricsPort = "telegraf.influxdata.com/port"
	// TelegrafMetricsPorts is used to configure which port telegraf should scrape, comma separated list of ports to scrape
	TelegrafMetricsPorts = "telegraf.influxdata.com/ports"
	// TelegrafMetricsPath is used to configure at which path to configure scraping to (a port must be configured also), will apply to all ports if multiple are configured
	TelegrafMetricsPath = "telegraf.influxdata.com/path"
	// TelegrafMetricsScheme is used to configure at the scheme for the metrics to scrape, will apply to all ports if multiple are configured
	TelegrafMetricsScheme = "telegraf.influxdata.com/scheme"
	// TelegrafInterval is used to configure interval for telegraf (Go style duration, e.g 5s, 30s, 2m .. )
	TelegrafInterval = "telegraf.influxdata.com/interval"
	// TelegrafRawInput is used to configure custom inputs for telegraf
	TelegrafRawInput = "telegraf.influxdata.com/inputs"
	// TelegrafEnableInternal enabled internal input plugins for
	TelegrafEnableInternal = "telegraf.influxdata.com/internal"
	// TelegrafClass configures which kind of class to use (classes are configured on the operator)
	TelegrafClass = "telegraf.influxdata.com/class"
	// TelegrafSecretEnv allows adding secrets to the telegraf sidecar in the form of environment variables
	TelegrafSecretEnv = "telegraf.influxdata.com/secret-env"
	// defaultTelegrafImage is the default telegraf image
	defaultTelegrafImage = "docker.io/library/telegraf:1.12"
	telegrafSecretPrefix = "telegraf-config"
)

// This function check if the pod have the correct annotations, otherwise the controller will skip this pod entirely
func skip(pod *corev1.Pod) bool {
	for key := range pod.GetAnnotations() {
		if strings.Contains(key, TelegrafAnnotationCommon) {
			return false
		}
	}
	return true
}

func addSidecar(pod *corev1.Pod, name, namespace, telegrafConf string) (*corev1.Secret, error) {
	pod.Spec.Containers = append(pod.Spec.Containers, newContainer(pod))
	pod.Spec.Volumes = append(pod.Spec.Volumes, newVolume(name))
	return newSecret(pod, name, namespace, telegrafConf)
}

// Assembling telegraf configuration
func assembleConf(pod *corev1.Pod, classData string) (config string, err error) {
	ports := ports(pod)
	if len(ports) != 0 {
		path := "/metrics"
		if extPath, ok := pod.Annotations[TelegrafMetricsPath]; ok {
			path = extPath
		}
		scheme := "http"
		if extScheme, ok := pod.Annotations[TelegrafMetricsScheme]; ok {
			scheme = extScheme
		}
		intervalConfig := ""
		intervalRaw, ok := pod.Annotations[TelegrafInterval]
		if ok {
			intervalConfig = fmt.Sprintf("interval = \"%s\"", intervalRaw)
		}
		urls := []string{}
		for _, port := range ports {
			urls = append(urls, fmt.Sprintf("%s://127.0.0.1:%s%s", scheme, port, path))
		}
		if len(urls) != 0 {
			config = fmt.Sprintf("%s\n%s", config, fmt.Sprintf("[[inputs.prometheus]]\n  urls = [\"%s\"]\n  %s\n", strings.Join(urls, `", "`), intervalConfig))
		}
	}
	if internalRaw, ok := pod.Annotations[TelegrafEnableInternal]; ok {
		internal, err := strconv.ParseBool(internalRaw)
		if err != nil {
			internal = false
		}
		if internal {
			config = fmt.Sprintf("%s\n%s", config, fmt.Sprintf("[[inputs.internal]]\n"))
		}
	}
	if inputsRaw, ok := pod.Annotations[TelegrafRawInput]; ok {
		config = fmt.Sprintf("%s\n%s", config, inputsRaw)
	}
	config = fmt.Sprintf("%s\n%s", config, classData)

	if _, err := toml.Parse([]byte(config)); err != nil {
		return "", fmt.Errorf("resulting Telegraf is not a valid file: %v", err)
	}

	return config, err
}

func newSecret(pod *corev1.Pod, name, namespace, telegrafConf string) (*corev1.Secret, error) {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", telegrafSecretPrefix, name),
			Namespace: namespace,
		},
		Type: "Opaque",
		StringData: map[string]string{
			"telegraf.conf": telegrafConf,
		},
	}, nil
}

func newVolume(name string) corev1.Volume {
	return corev1.Volume{
		Name: "telegraf-config",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: fmt.Sprintf("%s-%s", telegrafSecretPrefix, name),
			},
		},
	}
}

func newContainer(pod *corev1.Pod) corev1.Container {
	baseContainer := corev1.Container{
		Name:  "telegraf",
		Image: defaultTelegrafImage,
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				"cpu":    resource.MustParse("500m"),
				"memory": resource.MustParse("500Mi"),
			},
			Requests: corev1.ResourceList{
				"cpu":    resource.MustParse("50m"),
				"memory": resource.MustParse("50Mi"),
			},
		},
		Env: []corev1.EnvVar{
			{
				Name: "NODENAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
		},

		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "telegraf-config",
				MountPath: "/etc/telegraf",
			},
		},
	}
	if secretEnv, ok := pod.Annotations[TelegrafSecretEnv]; ok {
		baseContainer.EnvFrom = []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretEnv,
					},
					Optional: func(x bool) *bool { return &x }(true),
				},
			},
		}
	}
	return baseContainer
}

// ports gathers and merges unique ports from both TelegrafMetricsPort and TelegrafMetricsPorts.
func ports(pod *corev1.Pod) []string {
	uniquePorts := map[string]struct{}{}
	if p, ok := pod.Annotations[TelegrafMetricsPort]; ok {
		uniquePorts[p] = struct{}{}
	}
	if ports, ok := pod.Annotations[TelegrafMetricsPorts]; ok {
		for _, p := range strings.Split(ports, ",") {
			uniquePorts[p] = struct{}{}
		}
	}
	if len(uniquePorts) == 0 {
		return nil
	}

	ps := make([]string, 0, len(uniquePorts))
	for p := range uniquePorts {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}
