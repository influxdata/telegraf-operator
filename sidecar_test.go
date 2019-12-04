package main

import (
	"bytes"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	encode "k8s.io/apimachinery/pkg/runtime/serializer/json"
)

func Test_skip(t *testing.T) {
	withTelegraf := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				TelegrafInterval: "10s",
			},
		},
	}
	if skip(withTelegraf) {
		t.Errorf("pod %v should not be skipped", withTelegraf.GetAnnotations())
	}

	withoutTelegraf := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"something": "else",
			},
		},
	}
	if !skip(withoutTelegraf) {
		t.Errorf("pod %v should be skipped", withoutTelegraf.GetAnnotations())
	}
}

func Test_assembleConf(t *testing.T) {
	tests := []struct {
		name      string
		pod       *corev1.Pod
		classData string

		wantConfig string
		wantErr    bool
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
						TelegrafMetricsPorts: "6060,9999",
					},
				},
			},
			wantConfig: `
[[inputs.prometheus]]
  urls = ["http://127.0.0.1:6060/metrics", "http://127.0.0.1:9999/metrics"]
  

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
						TelegrafMetricsPorts:   "6060,9999",
						TelegrafEnableInternal: "true",
					},
				},
			},
			wantConfig: `
[[inputs.prometheus]]
  urls = ["https://127.0.0.1:6060/metrics/usage", "https://127.0.0.1:9999/metrics/usage"]
  interval = "10s"

[[inputs.internal]]

`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotConfig, err := assembleConf(tt.pod, tt.classData)
			if (err != nil) != tt.wantErr {
				t.Errorf("assembleConf() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotConfig != tt.wantConfig {
				t.Errorf("assembleConf() = %v, want %v", gotConfig, tt.wantConfig)
			}
		})
	}
}

func Test_addSidecar(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				TelegrafMetricsPorts: "6060",
			},
		},
	}

	secret, err := addSidecar(pod, "myname", "mynamespace", "")
	if err != nil {
		t.Errorf("unexpected error adding to sidecar: %v", err)
	}

	got := toYAML(t, secret)
	want := `apiVersion: v1
kind: Secret
metadata:
  creationTimestamp: null
  name: telegraf-config-myname
  namespace: mynamespace
stringData:
  telegraf.conf: "\n[[inputs.prometheus]]\n  urls = [\"http://127.0.0.1:6060/metrics\"]\n
    \ \n\n"
type: Opaque
`

	if got != want {
		t.Errorf("unexpected secret got:\n%v\nwant:\n%v", got, want)
	}

	got = toYAML(t, pod)
	want = `metadata:
  annotations:
    telegraf.influxdata.com/ports: "6060"
  creationTimestamp: null
spec:
  containers:
  - env:
    - name: NODENAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    image: docker.io/library/telegraf:1.12
    name: telegraf
    resources:
      limits:
        cpu: 500m
        memory: 500Mi
      requests:
        cpu: 50m
        memory: 50Mi
    volumeMounts:
    - mountPath: /etc/telegraf
      name: telegraf-config
  volumes:
  - name: telegraf-config
    secret:
      secretName: telegraf-config-myname
status: {}
`
	if got != want {
		t.Errorf("unexpected pod got:\n%v\nwant:\n%v", got, want)
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
						TelegrafMetricsPorts: "9999,6060,6060",
					},
				},
			},
			want: []string{"6060", "9999"},
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
