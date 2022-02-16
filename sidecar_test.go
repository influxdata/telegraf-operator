package main

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	logrTesting "github.com/go-logr/logr/testing"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	encode "k8s.io/apimachinery/pkg/runtime/serializer/json"
)

var (
	testEmptySecret = `
apiVersion: v1
kind: Secret
metadata:
  annotations:
    app.kubernetes.io/managed-by: telegraf-operator
  creationTimestamp: null
  labels:
    telegraf.influxdata.com/class: default
    telegraf.influxdata.com/pod: myname
  name: telegraf-config-myname
  namespace: mynamespace
stringData:
  telegraf.conf: |2+

type: Opaque`
	testEmptyIstioSecret = `
apiVersion: v1
kind: Secret
metadata:
  annotations:
    app.kubernetes.io/managed-by: telegraf-operator
  creationTimestamp: null
  labels:
    telegraf.influxdata.com/class: istio
    telegraf.influxdata.com/pod: myname
  name: telegraf-istio-config-myname
  namespace: mynamespace
stringData:
  telegraf.conf: |2-

      [[inputs.prometheus]]
        urls = ["http://127.0.0.1:15090/stats/prometheus"]


    # istio outputs
type: Opaque`
)

type mockClassDataHandler struct {
	classes map[string]string
}

func newMockClassDataHandler(classes map[string]string) *mockClassDataHandler {
	return &mockClassDataHandler{classes: classes}
}

func (h *mockClassDataHandler) validateClassData() error {
	return nil
}

func (m *mockClassDataHandler) getData(className string) (string, error) {
	v, ok := m.classes[className]
	if ok {
		return v, nil
	} else {
		return "", fmt.Errorf("class %s not found", className)
	}
}

func Test_skip(t *testing.T) {
	handler := &sidecarHandler{
		RequestsCPU:    defaultRequestsCPU,
		RequestsMemory: defaultRequestsMemory,
		LimitsCPU:      defaultLimitsCPU,
		LimitsMemory:   defaultLimitsMemory,
	}
	withTelegraf := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				TelegrafInterval: "10s",
			},
		},
	}
	if handler.skip(withTelegraf) {
		t.Errorf("pod %v should not be skipped", withTelegraf.GetAnnotations())
	}

	withoutTelegraf := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"something": "else",
			},
		},
	}
	if !handler.skip(withoutTelegraf) {
		t.Errorf("pod %v should be skipped", withoutTelegraf.GetAnnotations())
	}
}

func Test_validateRequestsAndLimits(t *testing.T) {
	tests := []struct {
		name    string
		sidecar *sidecarHandler
		wantErr bool
	}{
		{
			name: "validate default values",
			sidecar: &sidecarHandler{
				RequestsCPU:    defaultRequestsCPU,
				RequestsMemory: defaultRequestsMemory,
				LimitsCPU:      defaultLimitsCPU,
				LimitsMemory:   defaultLimitsMemory,
			},
			wantErr: false,
		},
		{
			name: "validate incorrect values",
			sidecar: &sidecarHandler{
				RequestsCPU:    "100x",
				RequestsMemory: "100x",
				LimitsCPU:      "100x",
				LimitsMemory:   "100x",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.sidecar.Logger = &logrTesting.TestLogger{T: t}

			err := tt.sidecar.validateRequestsAndLimits()
			if tt.wantErr && err == nil {
				t.Errorf("expected an error, but none was reported")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error received: %v", err)
			}
		})
	}
}

func Test_assembleConf(t *testing.T) {
	tests := []struct {
		name                        string
		pod                         *corev1.Pod
		classData                   string
		enableDefaultInternalPlugin bool
		wantConfig                  string
		wantErr                     bool
	}{
		{
			name: "default prometheus settings",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafMetricsPort: "6060",
					},
				},
			},
			wantConfig: `
[[inputs.prometheus]]
  urls = ["http://127.0.0.1:6060/metrics"]
  

`,
		},
		{
			name: "default prometheus settings with multiple ports",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafMetricsPorts: "6060,8086",
					},
				},
			},
			wantConfig: `
[[inputs.prometheus]]
  urls = ["http://127.0.0.1:6060/metrics", "http://127.0.0.1:8086/metrics"]
  

`,
		},
		{
			name: "default prometheus settings with raw input",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafMetricsPorts: "6060",
						TelegrafRawInput: `[global_tags]
  dc = "us-east-1"`,
					},
				},
			},
			wantConfig: `
[[inputs.prometheus]]
  urls = ["http://127.0.0.1:6060/metrics"]
  

[global_tags]
  dc = "us-east-1"
`,
		},
		{
			name: "all prometheus settings with internal",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafMetricsPath:    "/metrics/usage",
						TelegrafMetricsScheme:  "https",
						TelegrafInterval:       "10s",
						TelegrafMetricsPorts:   "6060,8086",
						TelegrafEnableInternal: "true",
					},
				},
			},
			wantConfig: `
[[inputs.prometheus]]
  urls = ["https://127.0.0.1:6060/metrics/usage", "https://127.0.0.1:8086/metrics/usage"]
  interval = "10s"

[[inputs.internal]]

`,
		},
		{
			name: "valid TOML syntax",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafRawInput: `
[[inputs.exec]]
  commands = []
`,
					},
				},
			},
			wantConfig: `
[[inputs.exec]]
  commands = []
`,
		},
		{
			name: "invalid TOML syntax",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafRawInput: `
[[inputs.invalid]]
  "invalid" = invalid
`,
					},
				},
			},
			wantErr: true,
		},
		{
			name:                        "validate enable default internal plugin",
			enableDefaultInternalPlugin: true,
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafClass: "default",
					},
				},
			},
			wantConfig: `
[[inputs.internal]]
`,
		},
		{
			name: "handle global_tags",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafGlobalTagLiteralPrefix + "foo": "bar",
					},
				},
			},
			wantConfig: `[global_tags]
  foo = "bar"`,
		},
		{
			name: "handle global_tags multiple",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafGlobalTagLiteralPrefix + "foo": "bar",
						TelegrafGlobalTagLiteralPrefix + "baz": "quz",
					},
				},
			},
			wantConfig: `[global_tags]
  baz = "quz"
  foo = "bar"`,
		},
		{
			name: "handle global_tags existing",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafRawInput:                       "[global_tags]\n  a = \"b\"",
						TelegrafGlobalTagLiteralPrefix + "foo": "bar",
					},
				},
			},
			wantConfig: `[global_tags]
  foo = "bar"
  a = "b"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			handler := &sidecarHandler{
				ClassDataHandler:            newMockClassDataHandler(map[string]string{"class": tt.classData}),
				EnableDefaultInternalPlugin: tt.enableDefaultInternalPlugin,
				RequestsCPU:                 defaultRequestsCPU,
				RequestsMemory:              defaultRequestsMemory,
				LimitsCPU:                   defaultLimitsCPU,
				LimitsMemory:                defaultLimitsMemory,
				Logger:                      &logrTesting.TestLogger{T: t},
			}
			gotConfig, err := handler.assembleConf(tt.pod, "class")
			if (err != nil) != tt.wantErr {
				t.Errorf("assembleConf() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if strings.TrimSpace(gotConfig) != strings.TrimSpace(tt.wantConfig) {
				t.Errorf("assembleConf() = %v, want %v", gotConfig, tt.wantConfig)
			}
		})
	}
}

func Test_addSidecars(t *testing.T) {
	tests := []struct {
		name                        string
		pod                         *corev1.Pod
		enableDefaultInternalPlugin bool
		enableIstioInjection        bool
		telegrafWatchConfig         string
		istioTelegrafImage          string
		istioOutputClass            string
		wantSecrets                 []string
		wantPod                     string
	}{
		{
			name: "validate prometheus inputs creation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafMetricsPorts: "6060",
					},
				},
			},
			wantSecrets: []string{
				`apiVersion: v1
kind: Secret
metadata:
  annotations:
    app.kubernetes.io/managed-by: telegraf-operator
  creationTimestamp: null
  labels:
    telegraf.influxdata.com/class: default
    telegraf.influxdata.com/pod: myname
  name: telegraf-config-myname
  namespace: mynamespace
stringData:
  telegraf.conf: "\n[[inputs.prometheus]]\n  urls = [\"http://127.0.0.1:6060/metrics\"]\n  \n\n"
type: Opaque`,
			},
		},
		{
			name: "validate default telegraf pod definition",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafClass: "default",
					},
				},
			},
			wantPod: `
metadata:
  annotations:
    telegraf.influxdata.com/class: default
  creationTimestamp: null
spec:
  containers:
  - command:
    - telegraf
    - --config
    - /etc/telegraf/telegraf.conf
    env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    image: docker.io/library/telegraf:1.19
    name: telegraf
    resources:
      limits:
        cpu: 200m
        memory: 200Mi
      requests:
        cpu: 10m
        memory: 10Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-config
  volumes:
  - name: telegraf-config
    secret:
      secretName: telegraf-config-myname
status: {}
      `,
			wantSecrets: []string{testEmptySecret},
		},
		{
			name: "validate custom telegraf image pod definition",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafImage: "docker.io/library/telegraf:1.11",
					},
				},
			},
			wantPod: `
metadata:
  annotations:
    telegraf.influxdata.com/image: docker.io/library/telegraf:1.11
  creationTimestamp: null
spec:
  containers:
  - command:
    - telegraf
    - --config
    - /etc/telegraf/telegraf.conf
    env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    image: docker.io/library/telegraf:1.11
    name: telegraf
    resources:
      limits:
        cpu: 200m
        memory: 200Mi
      requests:
        cpu: 10m
        memory: 10Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-config
  volumes:
  - name: telegraf-config
    secret:
      secretName: telegraf-config-myname
status: {}
      `,
			wantSecrets: []string{testEmptySecret},
		},
		{
			name: "validate enable default internal plugin",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafClass: "default",
					},
				},
			},
			enableDefaultInternalPlugin: true,
			wantSecrets: []string{
				`apiVersion: v1
kind: Secret
metadata:
  annotations:
    app.kubernetes.io/managed-by: telegraf-operator
  creationTimestamp: null
  labels:
    telegraf.influxdata.com/class: default
    telegraf.influxdata.com/pod: myname
  name: telegraf-config-myname
  namespace: mynamespace
stringData:
  telegraf.conf: |2+

    [[inputs.internal]]

type: Opaque`,
			},
		},
		{
			name: "validate custom resources and limits",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafRequestsCPU:    "100m",
						TelegrafRequestsMemory: "100Mi",
						TelegrafLimitsCPU:      "400m",
						TelegrafLimitsMemory:   "400Mi",
					},
				},
			},
			wantPod: `
metadata:
  annotations:
    telegraf.influxdata.com/limits-cpu: 400m
    telegraf.influxdata.com/limits-memory: 400Mi
    telegraf.influxdata.com/requests-cpu: 100m
    telegraf.influxdata.com/requests-memory: 100Mi
  creationTimestamp: null
spec:
  containers:
  - command:
    - telegraf
    - --config
    - /etc/telegraf/telegraf.conf
    env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    image: docker.io/library/telegraf:1.19
    name: telegraf
    resources:
      limits:
        cpu: 400m
        memory: 400Mi
      requests:
        cpu: 100m
        memory: 100Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-config
  volumes:
  - name: telegraf-config
    secret:
      secretName: telegraf-config-myname
status: {}
      `,
			wantSecrets: []string{testEmptySecret},
		},
		{
			name: "validate incorrect resources to fallback default resources",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafRequestsCPU: "100x",
						TelegrafLimitsCPU:   "750m",
					},
				},
			},
			wantPod: `
metadata:
  annotations:
    telegraf.influxdata.com/limits-cpu: 750m
    telegraf.influxdata.com/requests-cpu: 100x
  creationTimestamp: null
spec:
  containers:
  - command:
    - telegraf
    - --config
    - /etc/telegraf/telegraf.conf
    env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    image: docker.io/library/telegraf:1.19
    name: telegraf
    resources:
      limits:
        cpu: 750m
        memory: 200Mi
      requests:
        cpu: 10m
        memory: 10Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-config
  volumes:
  - name: telegraf-config
    secret:
      secretName: telegraf-config-myname
status: {}
`,
			wantSecrets: []string{testEmptySecret},
		},
		{
			name: "validate incorrect resources to fallback default resources",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafRequestsCPU: "100x",
						TelegrafLimitsCPU:   "750m",
					},
				},
			},
			wantPod: `
metadata:
  annotations:
    telegraf.influxdata.com/limits-cpu: 750m
    telegraf.influxdata.com/requests-cpu: 100x
  creationTimestamp: null
spec:
  containers:
  - command:
    - telegraf
    - --config
    - /etc/telegraf/telegraf.conf
    env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    image: docker.io/library/telegraf:1.19
    name: telegraf
    resources:
      limits:
        cpu: 750m
        memory: 200Mi
      requests:
        cpu: 10m
        memory: 10Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-config
  volumes:
  - name: telegraf-config
    secret:
      secretName: telegraf-config-myname
status: {}
`,
			wantSecrets: []string{testEmptySecret},
		},
		{
			name: "does not add telegraf sidecar when container already exists",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafClass: "default",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "telegraf",
							Image: "alpine:latest",
						},
					},
				},
			},
			wantPod: `
metadata:
  annotations:
    telegraf.influxdata.com/class: default
  creationTimestamp: null
spec:
  containers:
  - image: alpine:latest
    name: telegraf
    resources: {}
status: {}
`,
			wantSecrets: []string{},
		},
		{
			name: "does not add istio sidecar when not enabled",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						IstioSidecarAnnotation: "dummy",
					},
				},
			},
			wantPod: `
metadata:
  annotations:
    sidecar.istio.io/status: dummy
  creationTimestamp: null
spec:
  containers: null
status: {}
`,
			wantSecrets: []string{},
		},
		{
			name: "adds istio sidecar when sidecar annotation enabled and istio injection enabled",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						IstioSidecarAnnotation: "dummy",
					},
				},
			},
			enableIstioInjection: true,
			istioOutputClass:     "istio",
			wantPod: `
metadata:
  annotations:
    sidecar.istio.io/status: dummy
  creationTimestamp: null
spec:
  containers:
  - command:
    - telegraf
    - --config
    - /etc/telegraf/telegraf.conf
    env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    image: docker.io/library/telegraf:1.19
    name: telegraf-istio
    resources:
      limits:
        cpu: 200m
        memory: 200Mi
      requests:
        cpu: 10m
        memory: 10Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-istio-config
  volumes:
  - name: telegraf-istio-config
    secret:
      secretName: telegraf-istio-config-myname
status: {}
`,
			wantSecrets: []string{testEmptyIstioSecret},
		},
		{
			name: "does not add istio sidecar when container already exists",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						IstioSidecarAnnotation: "dummy",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "telegraf-istio",
							Image: "alpine:latest",
						},
					},
				},
			},
			enableIstioInjection: true,
			istioOutputClass:     "istio",
			wantPod: `
metadata:
  annotations:
    sidecar.istio.io/status: dummy
  creationTimestamp: null
spec:
  containers:
  - image: alpine:latest
    name: telegraf-istio
    resources: {}
status: {}
`,
			wantSecrets: []string{},
		},
		{
			name: "adds istio sidecar when sidecar annotation enabled and istio injection enabled",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						IstioSidecarAnnotation: "dummy",
					},
				},
			},
			enableIstioInjection: true,
			istioTelegrafImage:   "docker.io/library/telegraf:1.11",
			istioOutputClass:     "istio",
			wantPod: `
metadata:
  annotations:
    sidecar.istio.io/status: dummy
  creationTimestamp: null
spec:
  containers:
  - command:
    - telegraf
    - --config
    - /etc/telegraf/telegraf.conf
    env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    image: docker.io/library/telegraf:1.11
    name: telegraf-istio
    resources:
      limits:
        cpu: 200m
        memory: 200Mi
      requests:
        cpu: 10m
        memory: 10Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-istio-config
  volumes:
  - name: telegraf-istio-config
    secret:
      secretName: telegraf-istio-config-myname
status: {}
`,
			wantSecrets: []string{testEmptyIstioSecret},
		},
		{
			name: "adds both regular telegraf and istio sidecars when sidecar annotation enabled and istio injection enabled",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						IstioSidecarAnnotation: "dummy",
						TelegrafClass:          "default",
					},
				},
			},
			enableIstioInjection: true,
			istioOutputClass:     "istio",
			wantPod: `
metadata:
  annotations:
    sidecar.istio.io/status: dummy
    telegraf.influxdata.com/class: default
  creationTimestamp: null
spec:
  containers:
  - command:
    - telegraf
    - --config
    - /etc/telegraf/telegraf.conf
    env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    image: docker.io/library/telegraf:1.19
    name: telegraf
    resources:
      limits:
        cpu: 200m
        memory: 200Mi
      requests:
        cpu: 10m
        memory: 10Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-config
  - command:
    - telegraf
    - --config
    - /etc/telegraf/telegraf.conf
    env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    image: docker.io/library/telegraf:1.19
    name: telegraf-istio
    resources:
      limits:
        cpu: 200m
        memory: 200Mi
      requests:
        cpu: 10m
        memory: 10Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-istio-config
  volumes:
  - name: telegraf-config
    secret:
      secretName: telegraf-config-myname
  - name: telegraf-istio-config
    secret:
      secretName: telegraf-istio-config-myname
status: {}
`,
			wantSecrets: []string{testEmptySecret, testEmptyIstioSecret},
		},
		{
			name: "sets config-watch value if specified",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafClass: "default",
					},
				},
			},
			enableIstioInjection: true,
			istioOutputClass:     "istio",
			telegrafWatchConfig:  "inotify",
			wantPod: `
metadata:
  annotations:
    telegraf.influxdata.com/class: default
  creationTimestamp: null
spec:
  containers:
  - command:
    - telegraf
    - --config
    - /etc/telegraf/telegraf.conf
    - --watch-config
    - inotify
    env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    image: docker.io/library/telegraf:1.19
    name: telegraf
    resources:
      limits:
        cpu: 200m
        memory: 200Mi
      requests:
        cpu: 10m
        memory: 10Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-config
  volumes:
  - name: telegraf-config
    secret:
      secretName: telegraf-config-myname
status: {}
`,
			wantSecrets: []string{testEmptySecret},
		},
		{
			name: "validate secret-env annotation usage creation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafSecretEnv: "mysecret",
					},
				},
			},
			wantPod: `
metadata:
  annotations:
    telegraf.influxdata.com/secret-env: mysecret
  creationTimestamp: null
spec:
  containers:
  - command:
    - telegraf
    - --config
    - /etc/telegraf/telegraf.conf
    env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    envFrom:
    - secretRef:
        name: mysecret
        optional: true
    image: docker.io/library/telegraf:1.19
    name: telegraf
    resources:
      limits:
        cpu: 200m
        memory: 200Mi
      requests:
        cpu: 10m
        memory: 10Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-config
  volumes:
  - name: telegraf-config
    secret:
      secretName: telegraf-config-myname
status: {}
      `,
			wantSecrets: []string{testEmptySecret},
		},
		{
			name: "validate env-fieldref- annotation usage creation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafEnvFieldRefPrefix + "NAMESPACE": "metadata.namespace",
					},
				},
			},
			wantPod: `
metadata:
  annotations:
    telegraf.influxdata.com/env-fieldref-NAMESPACE: metadata.namespace
  creationTimestamp: null
spec:
  containers:
  - command:
    - telegraf
    - --config
    - /etc/telegraf/telegraf.conf
    env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    - name: NAMESPACE
      valueFrom:
        fieldRef:
          fieldPath: metadata.namespace
    image: docker.io/library/telegraf:1.19
    name: telegraf
    resources:
      limits:
        cpu: 200m
        memory: 200Mi
      requests:
        cpu: 10m
        memory: 10Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-config
  volumes:
  - name: telegraf-config
    secret:
      secretName: telegraf-config-myname
status: {}
      `,
			wantSecrets: []string{testEmptySecret},
		},
		{
			name: "validate env-literal- annotation usage creation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafEnvLiteralPrefix + "STACK_VERSION": "1.0",
					},
				},
			},
			wantPod: `
metadata:
  annotations:
    telegraf.influxdata.com/env-literal-STACK_VERSION: "1.0"
  creationTimestamp: null
spec:
  containers:
  - command:
    - telegraf
    - --config
    - /etc/telegraf/telegraf.conf
    env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    - name: STACK_VERSION
      value: "1.0"
    image: docker.io/library/telegraf:1.19
    name: telegraf
    resources:
      limits:
        cpu: 200m
        memory: 200Mi
      requests:
        cpu: 10m
        memory: 10Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-config
  volumes:
  - name: telegraf-config
    secret:
      secretName: telegraf-config-myname
status: {}
      `,
			wantSecrets: []string{testEmptySecret},
		},
		{
			name: "validate env-configmapkeyref- annotation usage creation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafEnvConfigMapKeyRefPrefix + "VERSION": "configmap-name.application.version",
					},
				},
			},
			wantPod: `
metadata:
  annotations:
    telegraf.influxdata.com/env-configmapkeyref-VERSION: configmap-name.application.version
  creationTimestamp: null
spec:
  containers:
  - command:
    - telegraf
    - --config
    - /etc/telegraf/telegraf.conf
    env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    - name: VERSION
      valueFrom:
        configMapKeyRef:
          key: application.version
          name: configmap-name
    image: docker.io/library/telegraf:1.19
    name: telegraf
    resources:
      limits:
        cpu: 200m
        memory: 200Mi
      requests:
        cpu: 10m
        memory: 10Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-config
  volumes:
  - name: telegraf-config
    secret:
      secretName: telegraf-config-myname
status: {}
      `,
			wantSecrets: []string{testEmptySecret},
		},
		{
			name: "validate env-secretref- annotation usage creation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafEnvSecretKeyRefPrefix + "PASSWORD": "app-secret.user.password",
					},
				},
			},
			wantPod: `
metadata:
  annotations:
    telegraf.influxdata.com/env-secretkeyref-PASSWORD: app-secret.user.password
  creationTimestamp: null
spec:
  containers:
  - command:
    - telegraf
    - --config
    - /etc/telegraf/telegraf.conf
    env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    - name: PASSWORD
      valueFrom:
        secretKeyRef:
          key: user.password
          name: app-secret
    image: docker.io/library/telegraf:1.19
    name: telegraf
    resources:
      limits:
        cpu: 200m
        memory: 200Mi
      requests:
        cpu: 10m
        memory: 10Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-config
  volumes:
  - name: telegraf-config
    secret:
      secretName: telegraf-config-myname
status: {}
      `,
			wantSecrets: []string{testEmptySecret},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := createTempClassesDirectory(t, map[string]string{
				"default": "",
				"istio":   "# istio outputs",
			})
			defer os.RemoveAll(dir)

			logger := &logrTesting.TestLogger{T: t}

			testClassDataHandler := &directoryClassDataHandler{
				Logger:                   logger,
				TelegrafClassesDirectory: dir,
			}

			handler := &sidecarHandler{
				ClassDataHandler:            testClassDataHandler,
				TelegrafDefaultClass:        "default",
				TelegrafImage:               defaultTelegrafImage,
				TelegrafWatchConfig:         tt.telegrafWatchConfig,
				EnableDefaultInternalPlugin: tt.enableDefaultInternalPlugin,
				EnableIstioInjection:        tt.enableIstioInjection,
				IstioOutputClass:            tt.istioOutputClass,
				IstioTelegrafImage:          tt.istioTelegrafImage,
				RequestsCPU:                 defaultRequestsCPU,
				RequestsMemory:              defaultRequestsMemory,
				LimitsCPU:                   defaultLimitsCPU,
				LimitsMemory:                defaultLimitsMemory,
				Logger:                      &logrTesting.TestLogger{T: t},
			}

			result, err := handler.addSidecars(tt.pod, "myname", "mynamespace")
			if err != nil {
				t.Errorf("unexpected error adding to sidecar: %v", err)
			}

			if want, got := len(tt.wantSecrets), len(result.secrets); got != want {
				t.Errorf("invalid number of secrets returned got: %d; want: %d", got, want)
			}
			for i := 0; i < len(result.secrets); i++ {
				if want, got := strings.TrimSpace(tt.wantSecrets[i]), strings.TrimSpace(toYAML(t, result.secrets[i])); got != want {
					t.Errorf("unexpected secret %d got:\n%v\nwant:\n%v", i, got, want)
				}
			}

			if tt.wantPod != "" {
				if want, got := strings.TrimSpace(tt.wantPod), strings.TrimSpace(toYAML(t, tt.pod)); got != want {
					t.Errorf("unexpected pod got:\n%v\nwant:\n%v", got, want)
				}
			}
		})
	}
}

func Test_ports(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want []string
	}{
		{
			name: "ports merges ports for both annotations",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafMetricsPort:  "6060",
						TelegrafMetricsPorts: "6060,8080,8888",
					},
				},
			},
			want: []string{"6060", "8080", "8888"},
		},
		{
			name: "no annotation returns no ports",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
		},
		{
			name: "ports are unique and returned in order",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafMetricsPorts: "8086,6060,6060",
					},
				},
			},
			want: []string{"6060", "8086"},
		},
		{
			name: "single port from TelegrafMetricsPorts",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafMetricsPorts: "6060",
					},
				},
			},
			want: []string{"6060"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ports(tt.pod); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ports() = %v, want %v", got, tt.want)
			}
		})
	}
}

func toYAML(t *testing.T, o runtime.Object) string {
	t.Helper()

	enc := encode.NewYAMLSerializer(encode.DefaultMetaFactory, nil, nil)
	var b bytes.Buffer
	if err := enc.Encode(o, &b); err != nil {
		t.Errorf("unable to encode container %v", err)
	}

	return b.String()
}
