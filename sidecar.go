package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"github.com/influxdata/toml"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	istioInputsConf = `
  [[inputs.prometheus]]
    urls = ["http://127.0.0.1:15090/stats/prometheus"]
`
)

const (
	// IstioSidecarAnnotation is the annotation used by istio sidecar handler
	IstioSidecarAnnotation = "sidecar.istio.io/status"

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
	// TelegrafImage allows specifying a custom telegraf image to be used in the sidecar container
	TelegrafImage = "telegraf.influxdata.com/image"
	// TelegrafRequestsCPU allows specifying custom CPU resource requests
	TelegrafRequestsCPU = "telegraf.influxdata.com/requests-cpu"
	// TelegrafRequestsMemory allows specifying custom memory resource requests
	TelegrafRequestsMemory = "telegraf.influxdata.com/requests-memory"
	// TelegrafLimitsCPU allows specifying custom CPU resource limits
	TelegrafLimitsCPU = "telegraf.influxdata.com/limits-cpu"
	// TelegrafLimitsMemory allows specifying custom memory resource limits
	TelegrafLimitsMemory = "telegraf.influxdata.com/limits-memory"
	telegrafSecretInfix  = "config"

	TelegrafSecretAnnotationKey   = "app.kubernetes.io/managed-by"
	TelegrafSecretAnnotationValue = "telegraf-operator"
	TelegrafSecretDataKey         = "telegraf.conf"
)

type sidecarHandler struct {
	ClassDataHandler            *classDataHandler
	Logger                      logr.Logger
	TelegrafDefaultClass        string
	TelegrafImage               string
	EnableDefaultInternalPlugin bool
	RequestsCPU                 string
	RequestsMemory              string
	LimitsCPU                   string
	LimitsMemory                string
	EnableIstioInjection        bool
	IstioOutputClass            string
	IstioTelegrafImage          string
}

type sidecarHandlerResponse struct {
	// list of secrets to create alongside with the changes
	secrets []*corev1.Secret
}

// This function check if the pod have the correct annotations, otherwise the controller will skip this pod entirely
func (h *sidecarHandler) skip(pod *corev1.Pod) bool {
	return !h.shouldAddTelegrafSidecar(pod) && !h.shouldAddIstioTelegrafSidecar(pod)
}

func (h *sidecarHandler) shouldAddTelegrafSidecar(pod *corev1.Pod) bool {
	if podHasContainerName(pod, "telegraf") {
		return false
	}

	for key := range pod.GetAnnotations() {
		if strings.Contains(key, TelegrafAnnotationCommon) {
			return true
		}
	}

	return false
}

func (h *sidecarHandler) shouldAddIstioTelegrafSidecar(pod *corev1.Pod) bool {
	if podHasContainerName(pod, "telegraf-istio") {
		return false
	}

	if !h.EnableIstioInjection {
		return false
	}

	for key := range pod.GetAnnotations() {
		if key == IstioSidecarAnnotation {
			return true
		}
	}

	return false
}

func (h *sidecarHandler) validateRequestsAndLimits() error {
	if _, err := resource.ParseQuantity(h.RequestsCPU); err != nil {
		return err
	}
	if _, err := resource.ParseQuantity(h.RequestsMemory); err != nil {
		return err
	}
	if _, err := resource.ParseQuantity(h.LimitsCPU); err != nil {
		return err
	}
	if _, err := resource.ParseQuantity(h.LimitsMemory); err != nil {
		return err
	}

	return nil
}

func (h *sidecarHandler) telegrafSecretNames(name string) []string {
	return []string{
		fmt.Sprintf("telegraf-%s-%s", telegrafSecretInfix, name),
		fmt.Sprintf("telegraf-istio-%s-%s", telegrafSecretInfix, name),
	}
}

func (h *sidecarHandler) addSidecars(pod *corev1.Pod, name, namespace string) (*sidecarHandlerResponse, error) {
	result := &sidecarHandlerResponse{}
	if h.shouldAddTelegrafSidecar(pod) {
		err := h.addTelegrafSidecar(result, pod, name, namespace, "telegraf")
		if err != nil {
			return nil, err
		}
	}

	if h.shouldAddIstioTelegrafSidecar(pod) {
		err := h.addIstioTelegrafSidecar(result, pod, name, namespace)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (h *sidecarHandler) addTelegrafSidecar(result *sidecarHandlerResponse, pod *corev1.Pod, name, namespace, containerName string) error {
	className := h.TelegrafDefaultClass
	if extClass, ok := pod.Annotations[TelegrafClass]; ok {
		className = extClass
	}

	classData, err := h.ClassDataHandler.getData(className)
	if err != nil {
		return newNonFatalError(err, "telegraf-operator could not create sidecar container for unknown class")
	}

	telegrafConf, err := h.assembleConf(pod, classData)
	if err != nil {
		return newNonFatalError(err, "telegraf-operator could not create sidecar container due to error in class data")
	}

	container, err := h.newContainer(pod, containerName)
	if err != nil {
		return err
	}

	return h.addContainerAndSecret(result, pod, container, name, namespace, telegrafConf)
}

func (h *sidecarHandler) addIstioTelegrafSidecar(result *sidecarHandlerResponse, pod *corev1.Pod, name, namespace string) error {
	classData, err := h.ClassDataHandler.getData(h.IstioOutputClass)
	if err != nil {
		return newNonFatalError(err, "telegraf-operator could not create sidecar container for istio class")
	}

	telegrafConf := fmt.Sprintf("%s\n\n%s", istioInputsConf, classData)

	container, err := h.newIstioContainer(pod, "telegraf-istio")
	if err != nil {
		return err
	}

	return h.addContainerAndSecret(result, pod, container, name, namespace, telegrafConf)
}

func (h *sidecarHandler) addContainerAndSecret(result *sidecarHandlerResponse, pod *corev1.Pod, container corev1.Container, name, namespace, telegrafConf string) error {
	pod.Spec.Containers = append(pod.Spec.Containers, container)
	pod.Spec.Volumes = append(pod.Spec.Volumes, h.newVolume(name, container.Name))
	secret, err := h.newSecret(pod, name, namespace, container.Name, telegrafConf)
	if err != nil {
		return err
	}
	result.secrets = append(result.secrets, secret)

	return nil
}

// Assembling telegraf configuration
func (h *sidecarHandler) assembleConf(pod *corev1.Pod, classData string) (telegrafConf string, err error) {
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
			telegrafConf = fmt.Sprintf("%s\n%s", telegrafConf, fmt.Sprintf("[[inputs.prometheus]]\n  urls = [\"%s\"]\n  %s\n", strings.Join(urls, `", "`), intervalConfig))
		}
	}
	enableInternal := h.EnableDefaultInternalPlugin
	if internalRaw, ok := pod.Annotations[TelegrafEnableInternal]; ok {
		internal, err := strconv.ParseBool(internalRaw)
		if err != nil {
			internal = false
		} else {
			// only override enableInternal if the annotation was successfully parsed as a boolean
			enableInternal = internal
		}
	}
	if enableInternal {
		telegrafConf = fmt.Sprintf("%s\n%s", telegrafConf, fmt.Sprintf("[[inputs.internal]]\n"))
	}
	if inputsRaw, ok := pod.Annotations[TelegrafRawInput]; ok {
		telegrafConf = fmt.Sprintf("%s\n%s", telegrafConf, inputsRaw)
	}
	telegrafConf = fmt.Sprintf("%s\n%s", telegrafConf, classData)

	if _, err := toml.Parse([]byte(telegrafConf)); err != nil {
		return "", fmt.Errorf("resulting Telegraf is not a valid file: %v", err)
	}

	return telegrafConf, err
}

func (h *sidecarHandler) newSecret(pod *corev1.Pod, name, namespace, containerName, telegrafConf string) (*corev1.Secret, error) {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s", containerName, telegrafSecretInfix, name),
			Namespace: namespace,
			Annotations: map[string]string{
				TelegrafSecretAnnotationKey: TelegrafSecretAnnotationValue,
			},
		},
		Type: "Opaque",
		StringData: map[string]string{
			TelegrafSecretDataKey: telegrafConf,
		},
	}, nil
}

func (h *sidecarHandler) newVolume(name, containerName string) corev1.Volume {
	return corev1.Volume{
		Name: fmt.Sprintf("%s-config", containerName),
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: fmt.Sprintf("%s-%s-%s", containerName, telegrafSecretInfix, name),
			},
		},
	}
}

// parseCustomOrDefaultQuantity parses custom quantity from annotations,
// defaulting to quantity specified to the handler if the custom one is not valid
func (h *sidecarHandler) parseCustomOrDefaultQuantity(customQuantity string, defaultQuantity string) (quantity resource.Quantity, err error) {
	if quantity, err = resource.ParseQuantity(customQuantity); err != nil {
		h.Logger.Info(fmt.Sprintf("unable to parse resource \"%s\": %v", customQuantity, err))
		quantity, err = resource.ParseQuantity(defaultQuantity)
	}
	return quantity, err
}

func (h *sidecarHandler) newContainer(pod *corev1.Pod, containerName string) (corev1.Container, error) {
	var telegrafImage string
	var telegrafRequestsCPU string
	var telegrafRequestsMemory string
	var telegrafLimitsCPU string
	var telegrafLimitsMemory string

	if customTelegrafImage, ok := pod.Annotations[TelegrafImage]; ok {
		telegrafImage = customTelegrafImage
	} else {
		telegrafImage = h.TelegrafImage
	}
	if customTelegrafRequestsCPU, ok := pod.Annotations[TelegrafRequestsCPU]; ok {
		telegrafRequestsCPU = customTelegrafRequestsCPU
	} else {
		telegrafRequestsCPU = h.RequestsCPU
	}
	if customTelegrafRequestsMemory, ok := pod.Annotations[TelegrafRequestsMemory]; ok {
		telegrafRequestsMemory = customTelegrafRequestsMemory
	} else {
		telegrafRequestsMemory = h.RequestsMemory
	}
	if customTelegrafLimitsCPU, ok := pod.Annotations[TelegrafLimitsCPU]; ok {
		telegrafLimitsCPU = customTelegrafLimitsCPU
	} else {
		telegrafLimitsCPU = h.LimitsCPU
	}
	if customTelegrafLimitsMemory, ok := pod.Annotations[TelegrafLimitsMemory]; ok {
		telegrafLimitsMemory = customTelegrafLimitsMemory
	} else {
		telegrafLimitsMemory = h.LimitsMemory
	}

	var parsedRequestsCPU resource.Quantity
	var parsedRequestsMemory resource.Quantity
	var parsedLimitsCPU resource.Quantity
	var parsedLimitsMemory resource.Quantity
	var err error

	if parsedRequestsCPU, err = h.parseCustomOrDefaultQuantity(telegrafRequestsCPU, h.RequestsCPU); err != nil {
		return corev1.Container{}, err
	}
	if parsedRequestsMemory, err = h.parseCustomOrDefaultQuantity(telegrafRequestsMemory, h.RequestsMemory); err != nil {
		return corev1.Container{}, err
	}

	if parsedLimitsCPU, err = h.parseCustomOrDefaultQuantity(telegrafLimitsCPU, h.LimitsCPU); err != nil {
		return corev1.Container{}, err
	}
	if parsedLimitsMemory, err = h.parseCustomOrDefaultQuantity(telegrafLimitsMemory, h.LimitsMemory); err != nil {
		return corev1.Container{}, err
	}

	baseContainer := corev1.Container{
		Name:  containerName,
		Image: telegrafImage,
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				"cpu":    parsedLimitsCPU,
				"memory": parsedLimitsMemory,
			},
			Requests: corev1.ResourceList{
				"cpu":    parsedRequestsCPU,
				"memory": parsedRequestsMemory,
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
				Name:      fmt.Sprintf("%s-config", containerName),
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
	return baseContainer, nil
}

func (h *sidecarHandler) newIstioContainer(pod *corev1.Pod, containerName string) (corev1.Container, error) {
	var parsedRequestsCPU resource.Quantity
	var parsedRequestsMemory resource.Quantity
	var parsedLimitsCPU resource.Quantity
	var parsedLimitsMemory resource.Quantity
	var err error

	if parsedRequestsCPU, err = resource.ParseQuantity(h.RequestsCPU); err != nil {
		return corev1.Container{}, err
	}
	if parsedRequestsMemory, err = resource.ParseQuantity(h.RequestsMemory); err != nil {
		return corev1.Container{}, err
	}

	if parsedLimitsCPU, err = resource.ParseQuantity(h.LimitsCPU); err != nil {
		return corev1.Container{}, err
	}
	if parsedLimitsMemory, err = resource.ParseQuantity(h.LimitsMemory); err != nil {
		return corev1.Container{}, err
	}

	telegrafImage := h.IstioTelegrafImage
	if telegrafImage == "" {
		telegrafImage = h.TelegrafImage
	}

	baseContainer := corev1.Container{
		Name:  containerName,
		Image: telegrafImage,
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				"cpu":    parsedLimitsCPU,
				"memory": parsedLimitsMemory,
			},
			Requests: corev1.ResourceList{
				"cpu":    parsedRequestsCPU,
				"memory": parsedRequestsMemory,
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
				Name:      fmt.Sprintf("%s-config", containerName),
				MountPath: "/etc/telegraf",
			},
		},
	}

	return baseContainer, nil
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

func podHasContainerName(pod *corev1.Pod, name string) bool {
	for _, container := range pod.Spec.Containers {
		if container.Name == name {
			return true
		}
	}
	return false
}
