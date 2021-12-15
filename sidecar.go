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
	// TelegrafEnvFieldRefPrefix allows adding fieldref references to the telegraf sidecar in the form of an environment variable
	TelegrafEnvFieldRefPrefix = "telegraf.influxdata.com/env-fieldref-"
	// TelegrafEnvConfigMapKeyRefPrefix allows adding configmap key references to the telegraf sidecar in the form of an environment variable
	TelegrafEnvConfigMapKeyRefPrefix = "telegraf.influxdata.com/env-configmapkeyref-"
	// TelegrafEnvSecretKeyRefPrefix allows adding secret key references to the telegraf sidecar in the form of an environment variable
	TelegrafEnvSecretKeyRefPrefix = "telegraf.influxdata.com/env-secretkeyref-"
	// TelegrafEnvLiteralPrefix allows adding a literal to the telegraf sidecar in the form of an environment variable
	TelegrafEnvLiteralPrefix = "telegraf.influxdata.com/env-literal-"
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

	// TelegrafShareVolume
	TelegrafSharedVolume     = "telegraf.influxdata.com/shared-volume"
	TelegrafSharedVolumePath = "telegraf.influxdata.com/shared-volume-path"

	TelegrafSecretAnnotationKey   = "app.kubernetes.io/managed-by"
	TelegrafSecretAnnotationValue = "telegraf-operator"
	TelegrafSecretDataKey         = "telegraf.conf"
	TelegrafSecretLabelClassName  = TelegrafClass
	TelegrafSecretLabelPod        = "telegraf.influxdata.com/pod"
)

// sidecarHandler provides logic for handling telegraf sidecars and related secrets.
type sidecarHandler struct {
	ClassDataHandler            classDataHandler
	Logger                      logr.Logger
	TelegrafDefaultClass        string
	TelegrafImage               string
	TelegrafWatchConfig         string
	EnableDefaultInternalPlugin bool
	RequestsCPU                 string
	RequestsMemory              string
	LimitsCPU                   string
	LimitsMemory                string
	EnableIstioInjection        bool
	IstioOutputClass            string
	IstioTelegrafImage          string
	IstioTelegrafWatchConfig    string
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

	telegrafConf, err := h.assembleConf(pod, className)
	if err != nil {
		return newNonFatalError(err, "telegraf-operator could not create sidecar container due to error in class data")
	}

	container, err := h.newContainer(pod, containerName)
	if err != nil {
		return err
	}

	return h.addContainerAndSecret(result, pod, container, className, name, namespace, telegrafConf)
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

	return h.addContainerAndSecret(result, pod, container, h.IstioOutputClass, name, namespace, telegrafConf)
}

func (h *sidecarHandler) addContainerAndSecret(result *sidecarHandlerResponse, pod *corev1.Pod, container corev1.Container, className, name, namespace, telegrafConf string) error {
	pod.Spec.Containers = append(pod.Spec.Containers, container)
	pod.Spec.Volumes = append(pod.Spec.Volumes, h.newVolume(name, container.Name))
	secret, err := h.newSecret(pod, className, name, namespace, container.Name, telegrafConf)
	if err != nil {
		return err
	}
	result.secrets = append(result.secrets, secret)

	return nil
}

func (h *sidecarHandler) getClassData(className string) (string, error) {
	return h.ClassDataHandler.getData(className)
}

// Assembling telegraf configuration
func (h *sidecarHandler) assembleConf(pod *corev1.Pod, className string) (telegrafConf string, err error) {
	classData, err := h.ClassDataHandler.getData(className)
	if err != nil {
		return "", newNonFatalError(err, "telegraf-operator could not create sidecar container for unknown class")
	}

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

func (h *sidecarHandler) newSecret(pod *corev1.Pod, className, name, namespace, containerName, telegrafConf string) (*corev1.Secret, error) {
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
			Labels: map[string]string{
				TelegrafSecretLabelClassName: className,
				TelegrafSecretLabelPod:       name,
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
	var telegrafSharedVolume string
	var telegrafSharedVolumePath string

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

	// For shared volume
	if customSharedVolume, ok := pod.Annotations[telegrafSharedVolume]; ok {
		telegrafSharedVolume = customSharedVolume
	}

	if customSharedVolumePath, ok := pod.Annotations[telegrafSharedVolumePath]; ok {
		telegrafSharedVolumePath = customSharedVolumePath
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

	if telegrafSharedVolume != "" && telegrafSharedVolumePath == "" {
		return corev1.Container{}, err
	}

	telegrafContainerCommand := createTelegrafCommand(h.TelegrafWatchConfig)

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      fmt.Sprintf("%s-config", containerName),
			MountPath: "/etc/telegraf",
		},
	}

	if telegrafSharedVolume != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      telegrafSharedVolume,
			MountPath: telegrafSharedVolumePath,
		})
	}

	baseContainer := corev1.Container{
		Name:    containerName,
		Image:   telegrafImage,
		Command: telegrafContainerCommand,
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

		VolumeMounts: volumeMounts,
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

	envFieldRef := AnnotationsWithPrefix(pod.Annotations, TelegrafEnvFieldRefPrefix)
	for name, fieldPath := range envFieldRef {
		baseContainer.Env = append(baseContainer.Env, corev1.EnvVar{
			Name: name,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fieldPath,
				},
			},
		})
	}

	literals := AnnotationsWithPrefix(pod.Annotations, TelegrafEnvLiteralPrefix)
	for name, value := range literals {
		baseContainer.Env = append(baseContainer.Env, corev1.EnvVar{
			Name:  name,
			Value: value,
		})
	}

	configMapKeyRefs := AnnotationsWithPrefix(pod.Annotations, TelegrafEnvConfigMapKeyRefPrefix)
	for name, value := range configMapKeyRefs {
		selector := strings.SplitN(value, ".", 2)
		if len(selector) == 2 {
			baseContainer.Env = append(baseContainer.Env, corev1.EnvVar{
				Name: name,
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: selector[0],
						},
						Key: selector[1],
					},
				},
			})
		} else {
			h.Logger.Info("unable to parse configmapkeyref %s with value of \"%s\"", name, value)
		}
	}

	secretKeyRefs := AnnotationsWithPrefix(pod.Annotations, TelegrafEnvSecretKeyRefPrefix)
	for name, value := range secretKeyRefs {
		selector := strings.SplitN(value, ".", 2)
		if len(selector) == 2 {
			baseContainer.Env = append(baseContainer.Env, corev1.EnvVar{
				Name: name,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: selector[0],
						},
						Key: selector[1],
					},
				},
			})
		} else {
			h.Logger.Info("unable to parse secretkeyref %s with value of \"%s\"", name, value)
		}
	}
	return baseContainer, nil
}

func AnnotationsWithPrefix(annotations map[string]string, prefix string) map[string]string {
	filtered := make(map[string]string)
	for k, v := range annotations {
		if strings.HasPrefix(k, prefix) {
			filtered[strings.TrimPrefix(k, prefix)] = v
		}
	}
	return filtered
}

func (h *sidecarHandler) newIstioContainer(pod *corev1.Pod, containerName string) (corev1.Container, error) {
	var parsedRequestsCPU resource.Quantity
	var parsedRequestsMemory resource.Quantity
	var parsedLimitsCPU resource.Quantity
	var parsedLimitsMemory resource.Quantity

	var telegrafSharedVolume string
	var telegrafSharedVolumePath string
	var err error

	if customSharedVolume, ok := pod.Annotations[telegrafSharedVolume]; ok {
		telegrafSharedVolume = customSharedVolume
	}

	if customSharedVolumePath, ok := pod.Annotations[telegrafSharedVolumePath]; ok {
		telegrafSharedVolumePath = customSharedVolumePath
	}

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

	if telegrafSharedVolume != "" && telegrafSharedVolumePath == "" {
		return corev1.Container{}, err
	}

	telegrafImage := h.IstioTelegrafImage
	if telegrafImage == "" {
		telegrafImage = h.TelegrafImage
	}

	telegrafContainerCommand := createTelegrafCommand(h.IstioTelegrafWatchConfig)

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      fmt.Sprintf("%s-config", containerName),
			MountPath: "/etc/telegraf",
		},
	}

	if telegrafSharedVolume != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      telegrafSharedVolume,
			MountPath: telegrafSharedVolumePath,
		})
	}

	baseContainer := corev1.Container{
		Name:    containerName,
		Image:   telegrafImage,
		Command: telegrafContainerCommand,
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

		VolumeMounts: volumeMounts,
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

func createTelegrafCommand(watchConfig string) []string {
	command := []string{"telegraf", "--config", "/etc/telegraf/telegraf.conf"}
	if watchConfig != "" {
		command = append(command, "--watch-config", watchConfig)
	}
	return command
}
