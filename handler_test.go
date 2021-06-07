package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"testing"

	admv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	logrTesting "github.com/go-logr/logr/testing"
)

const (
	testTelegrafClass = "testclass"
)

func createTempClassesDirectory(t *testing.T, classes map[string]string) string {
	dir, err := ioutil.TempDir("", "tests")
	if err != nil {
		t.Fatalf("unable to create temporary directory: %v", err)
	}
	for key, val := range classes {
		if err := ioutil.WriteFile(filepath.Join(dir, key), []byte(val), 0600); err != nil {
			t.Fatalf("unable to write temporary file: %v", err)
		}
	}

	return dir
}

func Test_podInjector_Handle(t *testing.T) {
	type want struct {
		Patches []string
		Allowed bool
		Code    int32
		Message string
	}
	type fields struct {
		TelegrafDefaultClass string
	}
	tests := []struct {
		name    string
		fields  fields
		objects []runtime.Object
		classes map[string]string
		req     admission.Request
		want    want
		handler *sidecarHandler
	}{
		{
			name: "error if no pod in request",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{},
			},
			want: want{
				Allowed: false,
				Code:    http.StatusBadRequest,
				Message: "there is no content to decode",
			},
		},
		{
			name: "skip pod if annotation not present",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Object: runtime.RawExtension{
						Raw: []byte(`{
								"apiVersion": "v1",
								"kind": "Pod",
								"metadata": {
								  "name": "simple"
								},
								"spec": {
								  "containers": [
									{
									  "name": "busybox",
									  "image": "busybox",
									  "args": [
										"sleep",
										"1000000"
									  ]
									}
								  ]
								}
							  }`),
					},
				},
			},
			want: want{
				Allowed: true,
				Code:    http.StatusOK,
			},
		},
		{
			name: "no sidecar added if secrets are not found",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Object: runtime.RawExtension{
						Raw: []byte(`{
								"apiVersion": "v1",
								"kind": "Pod",
								"metadata": {
								  "name": "simple",
								  "annotations": {
									"telegraf.influxdata.com/port": "8080",
									"telegraf.influxdata.com/path": "/v1/metrics",
									"telegraf.influxdata.com/interval": "5s"
								  }
								},
								"spec": {
								  "containers": [
									{
									  "name": "busybox",
									  "image": "busybox",
									  "args": [
										"sleep",
										"1000000"
									  ]
									}
								  ]
								}
							  }`),
					},
				},
			},
			want: want{
				Allowed: true,
				Code:    http.StatusOK,
			},
		},
		{
			name: "no sidecar added if invalid TOML configuration passed",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Object: runtime.RawExtension{
						Raw: []byte(`{
								"apiVersion": "v1",
								"kind": "Pod",
								"metadata": {
								  "name": "simple",
								  "annotations": {
									"telegraf.influxdata.com/port": "8080",
									"telegraf.influxdata.com/path": "/v1/metrics",
									"telegraf.influxdata.com/inputs": "[inputs.invalid]\n\"invalid\"=invalid"
								  }
								},
								"spec": {
								  "containers": [
									{
									  "name": "busybox",
									  "image": "busybox",
									  "args": [
										"sleep",
										"1000000"
									  ]
									}
								  ]
								}
							  }`),
					},
				},
			},
			want: want{
				Allowed: true,
				Code:    http.StatusOK,
			},
		},
		{
			name: "inject telegraf into container",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Operation: admv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte(`{
								"apiVersion": "v1",
								"kind": "Pod",
								"metadata": {
								  "name": "simple",
								  "annotations": {
									"telegraf.influxdata.com/port": "8080",
									"telegraf.influxdata.com/path": "/v1/metrics",
									"telegraf.influxdata.com/interval": "5s"
								  }
								},
								"spec": {
								  "containers": [
									{
									  "name": "busybox",
									  "image": "busybox",
									  "args": [
										"sleep",
										"1000000"
									  ]
									}
								  ]
								}
							  }`),
					},
				},
			},
			fields: fields{
				TelegrafDefaultClass: testTelegrafClass,
			},
			classes: map[string]string{testTelegrafClass: sampleClassData},
			want: want{
				Allowed: true,
				Patches: []string{
					`{"op":"add","path":"/metadata/creationTimestamp"}`,
					`{"op":"add","path":"/spec/containers/0/resources","value":{}}`,
					`{"op":"add","path":"/spec/containers/1","value":{"env":[{"name":"NODENAME","valueFrom":{"fieldRef":{"fieldPath":"spec.nodeName"}}}],"image":"docker.io/library/telegraf:1.14","name":"telegraf","resources":{"limits":{"cpu":"200m","memory":"200Mi"},"requests":{"cpu":"10m","memory":"10Mi"}},"volumeMounts":[{"mountPath":"/etc/telegraf","name":"telegraf-config"}]}}`,
					`{"op":"add","path":"/spec/volumes","value":[{"name":"telegraf-config","secret":{"secretName":"telegraf-config-simple"}}]}`,
					`{"op":"add","path":"/status","value":{}}`,
				},
			},
		},
		{
			name: "inject telegraf with custom image passed as sidecar config into container",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Operation: admv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte(`{
								"apiVersion": "v1",
								"kind": "Pod",
								"metadata": {
								  "name": "simple",
								  "annotations": {
									"telegraf.influxdata.com/port": "8080",
									"telegraf.influxdata.com/path": "/v1/metrics",
									"telegraf.influxdata.com/interval": "5s"
								  }
								},
								"spec": {
								  "containers": [
									{
									  "name": "busybox",
									  "image": "busybox",
									  "args": [
										"sleep",
										"1000000"
									  ]
									}
								  ]
								}
							  }`),
					},
				},
			},
			handler: &sidecarHandler{
				TelegrafImage:  "docker.io/library/telegraf:1.11",
				RequestsCPU:    defaultRequestsCPU,
				RequestsMemory: defaultRequestsMemory,
				LimitsCPU:      defaultLimitsCPU,
				LimitsMemory:   defaultLimitsMemory,
			},
			fields: fields{
				TelegrafDefaultClass: testTelegrafClass,
			},
			classes: map[string]string{testTelegrafClass: sampleClassData},
			want: want{
				Allowed: true,
				Patches: []string{
					`{"op":"add","path":"/metadata/creationTimestamp"}`,
					`{"op":"add","path":"/spec/containers/0/resources","value":{}}`,
					`{"op":"add","path":"/spec/containers/1","value":{"env":[{"name":"NODENAME","valueFrom":{"fieldRef":{"fieldPath":"spec.nodeName"}}}],"image":"docker.io/library/telegraf:1.11","name":"telegraf","resources":{"limits":{"cpu":"200m","memory":"200Mi"},"requests":{"cpu":"10m","memory":"10Mi"}},"volumeMounts":[{"mountPath":"/etc/telegraf","name":"telegraf-config"}]}}`,
					`{"op":"add","path":"/spec/volumes","value":[{"name":"telegraf-config","secret":{"secretName":"telegraf-config-simple"}}]}`,
					`{"op":"add","path":"/status","value":{}}`,
				},
			},
		},
		{
			name: "inject telegraf with custom image into container",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Operation: admv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte(`{
								"apiVersion": "v1",
								"kind": "Pod",
								"metadata": {
								  "name": "simple",
								  "annotations": {
									"telegraf.influxdata.com/port": "8080",
									"telegraf.influxdata.com/path": "/v1/metrics",
									"telegraf.influxdata.com/interval": "5s",
									"telegraf.influxdata.com/image": "docker.io/library/telegraf:1.11"
								  }
								},
								"spec": {
								  "containers": [
									{
									  "name": "busybox",
									  "image": "busybox",
									  "args": [
										"sleep",
										"1000000"
									  ]
									}
								  ]
								}
							  }`),
					},
				},
			},
			fields: fields{
				TelegrafDefaultClass: testTelegrafClass,
			},
			classes: map[string]string{testTelegrafClass: sampleClassData},
			want: want{
				Allowed: true,
				Patches: []string{
					`{"op":"add","path":"/metadata/creationTimestamp"}`,
					`{"op":"add","path":"/spec/containers/0/resources","value":{}}`,
					`{"op":"add","path":"/spec/containers/1","value":{"env":[{"name":"NODENAME","valueFrom":{"fieldRef":{"fieldPath":"spec.nodeName"}}}],"image":"docker.io/library/telegraf:1.11","name":"telegraf","resources":{"limits":{"cpu":"200m","memory":"200Mi"},"requests":{"cpu":"10m","memory":"10Mi"}},"volumeMounts":[{"mountPath":"/etc/telegraf","name":"telegraf-config"}]}}`,
					`{"op":"add","path":"/spec/volumes","value":[{"name":"telegraf-config","secret":{"secretName":"telegraf-config-simple"}}]}`,
					`{"op":"add","path":"/status","value":{}}`,
				},
			},
		},
		{
			name: "update telegraf container on inject",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Operation: admv1.Update,
					Object: runtime.RawExtension{
						Raw: []byte(`{
									"apiVersion": "v1",
									"kind": "Pod",
									"metadata": {
									  "name": "simple",
									  "annotations": {
										"telegraf.influxdata.com/port": "8080",
										"telegraf.influxdata.com/path": "/v1/metrics",
										"telegraf.influxdata.com/interval": "5s"
									  }
									},
									"spec": {
									  "containers": [
										{
										  "name": "busybox",
										  "image": "busybox",
										  "args": [
											"sleep",
											"1000000"
										  ]
										}
									  ]
									}
								  }`),
					},
				},
			},
			fields: fields{
				TelegrafDefaultClass: testTelegrafClass,
			},
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "telegraf-config-simple",
					},
					Type: "Opaque",
					Data: map[string][]byte{TelegrafSecretDataKey: []byte(sampleClassData)},
				},
			},
			classes: map[string]string{testTelegrafClass: sampleClassData},
			want: want{
				Allowed: true,
				Patches: []string{
					`{"op":"add","path":"/metadata/creationTimestamp"}`,
					`{"op":"add","path":"/spec/containers/0/resources","value":{}}`,
					`{"op":"add","path":"/spec/containers/1","value":{"env":[{"name":"NODENAME","valueFrom":{"fieldRef":{"fieldPath":"spec.nodeName"}}}],"image":"docker.io/library/telegraf:1.14","name":"telegraf","resources":{"limits":{"cpu":"200m","memory":"200Mi"},"requests":{"cpu":"10m","memory":"10Mi"}},"volumeMounts":[{"mountPath":"/etc/telegraf","name":"telegraf-config"}]}}`,
					`{"op":"add","path":"/spec/volumes","value":[{"name":"telegraf-config","secret":{"secretName":"telegraf-config-simple"}}]}`,
					`{"op":"add","path":"/status","value":{}}`,
				},
			},
		},
		{
			name: "delete telegraf secret",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Operation: admv1.Delete,
					Name:      "simple",
				},
			},
			fields: fields{
				TelegrafDefaultClass: testTelegrafClass,
			},
			classes: map[string]string{testTelegrafClass: sampleClassData},
			want: want{
				Code:    http.StatusOK,
				Allowed: true,
			},
		},
		{
			name: "inject telegraf with custom image into container",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Operation: admv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte(`{
								"apiVersion": "v1",
								"kind": "Pod",
								"metadata": {
								  "name": "simple",
								  "annotations": {
									"telegraf.influxdata.com/port": "8080",
									"telegraf.influxdata.com/path": "/v1/metrics",
									"telegraf.influxdata.com/interval": "5s",
									"telegraf.influxdata.com/image": "docker.io/library/telegraf:1.11"
								  }
								},
								"spec": {
								  "containers": [
									{
									  "name": "busybox",
									  "image": "busybox",
									  "args": [
										"sleep",
										"1000000"
									  ]
									}
								  ]
								}
							  }`),
					},
				},
			},
			fields: fields{
				TelegrafDefaultClass: testTelegrafClass,
			},
			classes: map[string]string{testTelegrafClass: sampleClassData},
			want: want{
				Allowed: true,
				Patches: []string{
					`{"op":"add","path":"/metadata/creationTimestamp"}`,
					`{"op":"add","path":"/spec/containers/0/resources","value":{}}`,
					`{"op":"add","path":"/spec/containers/1","value":{"env":[{"name":"NODENAME","valueFrom":{"fieldRef":{"fieldPath":"spec.nodeName"}}}],"image":"docker.io/library/telegraf:1.11","name":"telegraf","resources":{"limits":{"cpu":"200m","memory":"200Mi"},"requests":{"cpu":"10m","memory":"10Mi"}},"volumeMounts":[{"mountPath":"/etc/telegraf","name":"telegraf-config"}]}}`,
					`{"op":"add","path":"/spec/volumes","value":[{"name":"telegraf-config","secret":{"secretName":"telegraf-config-simple"}}]}`,
					`{"op":"add","path":"/status","value":{}}`,
				},
			},
		},
		{
			name: "accept custom requests CPU",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Operation: admv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte(`{
								"apiVersion": "v1",
								"kind": "Pod",
								"metadata": {
								  "name": "simple",
								  "annotations": {
									"telegraf.influxdata.com/port": "8080",
									"telegraf.influxdata.com/path": "/v1/metrics",
									"telegraf.influxdata.com/interval": "5s",
									"telegraf.influxdata.com/requests-cpu": "10m"
								  }
								},
								"spec": {
								  "containers": [
									{
									  "name": "busybox",
									  "image": "busybox",
									  "args": [
										"sleep",
										"1000000"
									  ]
									}
								  ]
								}
							  }`),
					},
				},
			},
			fields: fields{
				TelegrafDefaultClass: testTelegrafClass,
			},
			classes: map[string]string{testTelegrafClass: sampleClassData},
			want: want{
				Allowed: true,
				Patches: []string{
					`{"op":"add","path":"/metadata/creationTimestamp"}`,
					`{"op":"add","path":"/spec/containers/0/resources","value":{}}`,
					`{"op":"add","path":"/spec/containers/1","value":{"env":[{"name":"NODENAME","valueFrom":{"fieldRef":{"fieldPath":"spec.nodeName"}}}],"image":"docker.io/library/telegraf:1.14","name":"telegraf","resources":{"limits":{"cpu":"200m","memory":"200Mi"},"requests":{"cpu":"10m","memory":"10Mi"}},"volumeMounts":[{"mountPath":"/etc/telegraf","name":"telegraf-config"}]}}`,
					`{"op":"add","path":"/spec/volumes","value":[{"name":"telegraf-config","secret":{"secretName":"telegraf-config-simple"}}]}`,
					`{"op":"add","path":"/status","value":{}}`,
				},
			},
		},
		{
			name: "accept invalid custom requests CPU and fall back to default",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Operation: admv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte(`{
								"apiVersion": "v1",
								"kind": "Pod",
								"metadata": {
								  "name": "simple",
								  "annotations": {
									"telegraf.influxdata.com/port": "8080",
									"telegraf.influxdata.com/path": "/v1/metrics",
									"telegraf.influxdata.com/interval": "5s",
									"telegraf.influxdata.com/limits-memory": "800x",
									"telegraf.influxdata.com/limits-cpu": "750m"
								  }
								},
								"spec": {
								  "containers": [
									{
									  "name": "busybox",
									  "image": "busybox",
									  "args": [
										"sleep",
										"1000000"
									  ]
									}
								  ]
								}
							  }`),
					},
				},
			},
			fields: fields{
				TelegrafDefaultClass: testTelegrafClass,
			},
			classes: map[string]string{testTelegrafClass: sampleClassData},
			want: want{
				// TODO: clean up
				Allowed: true,
				Patches: []string{
					`{"op":"add","path":"/metadata/creationTimestamp"}`,
					`{"op":"add","path":"/spec/containers/0/resources","value":{}}`,
					`{"op":"add","path":"/spec/containers/1","value":{"env":[{"name":"NODENAME","valueFrom":{"fieldRef":{"fieldPath":"spec.nodeName"}}}],"image":"docker.io/library/telegraf:1.14","name":"telegraf","resources":{"limits":{"cpu":"750m","memory":"200Mi"},"requests":{"cpu":"10m","memory":"10Mi"}},"volumeMounts":[{"mountPath":"/etc/telegraf","name":"telegraf-config"}]}}`,
					`{"op":"add","path":"/spec/volumes","value":[{"name":"telegraf-config","secret":{"secretName":"telegraf-config-simple"}}]}`,
					`{"op":"add","path":"/status","value":{}}`,
				},
			},
		},
		{
			name: "skip telegraf into container if istio annotation present, but option enabled",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Operation: admv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte(`{
								"apiVersion": "v1",
								"kind": "Pod",
								"metadata": {
								  "name": "simple",
								  "annotations": {
									"sidecar.istio.io/status": "dummy"
								  }
								},
								"spec": {
								  "containers": [
									{
									  "name": "busybox",
									  "image": "busybox",
									  "args": [
										"sleep",
										"1000000"
									  ]
									}
								  ]
								}
							  }`),
					},
				},
			},
			fields: fields{
				TelegrafDefaultClass: testTelegrafClass,
			},
			classes: map[string]string{testTelegrafClass: sampleClassData},
			want: want{
				Allowed: true,
			},
		},
		{
			name: "inject telegraf-istio into container if option enabled",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Operation: admv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte(`{
								"apiVersion": "v1",
								"kind": "Pod",
								"metadata": {
								  "name": "simple",
								  "annotations": {
									"sidecar.istio.io/status": "dummy"
								  }
								},
								"spec": {
								  "containers": [
									{
									  "name": "busybox",
									  "image": "busybox",
									  "args": [
										"sleep",
										"1000000"
									  ]
									}
								  ]
								}
							  }`),
					},
				},
			},
			fields: fields{
				TelegrafDefaultClass: testTelegrafClass,
			},
			handler: &sidecarHandler{
				RequestsCPU:          defaultRequestsCPU,
				RequestsMemory:       defaultRequestsMemory,
				LimitsCPU:            defaultLimitsCPU,
				LimitsMemory:         defaultLimitsMemory,
				EnableIstioInjection: true,
				IstioOutputClass:     testTelegrafClass,
			},
			classes: map[string]string{testTelegrafClass: sampleClassData},
			want: want{
				Allowed: true,
				Patches: []string{
					`{"op":"add","path":"/metadata/creationTimestamp"}`,
					`{"op":"add","path":"/spec/containers/0/resources","value":{}}`,
					`{"op":"add","path":"/spec/containers/1","value":{"env":[{"name":"NODENAME","valueFrom":{"fieldRef":{"fieldPath":"spec.nodeName"}}}],"image":"docker.io/library/telegraf:1.14","name":"telegraf-istio","resources":{"limits":{"cpu":"200m","memory":"200Mi"},"requests":{"cpu":"10m","memory":"10Mi"}},"volumeMounts":[{"mountPath":"/etc/telegraf","name":"telegraf-istio-config"}]}}`,
					`{"op":"add","path":"/spec/volumes","value":[{"name":"telegraf-istio-config","secret":{"secretName":"telegraf-istio-config-simple"}}]}`,
					`{"op":"add","path":"/status","value":{}}`,
				},
			},
		},
		{
			name: "inject telegraf and telegraf-istio into container",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Operation: admv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte(`{
								"apiVersion": "v1",
								"kind": "Pod",
								"metadata": {
								  "name": "simple",
								  "annotations": {
									"sidecar.istio.io/status": "dummy",
									"telegraf.influxdata.com/port": "8080",
									"telegraf.influxdata.com/path": "/v1/metrics",
									"telegraf.influxdata.com/interval": "5s"
								  }
								},
								"spec": {
								  "containers": [
									{
									  "name": "busybox",
									  "image": "busybox",
									  "args": [
										"sleep",
										"1000000"
									  ]
									}
								  ]
								}
							  }`),
					},
				},
			},
			fields: fields{
				TelegrafDefaultClass: testTelegrafClass,
			},
			handler: &sidecarHandler{
				RequestsCPU:          defaultRequestsCPU,
				RequestsMemory:       defaultRequestsMemory,
				LimitsCPU:            defaultLimitsCPU,
				LimitsMemory:         defaultLimitsMemory,
				EnableIstioInjection: true,
				IstioOutputClass:     testTelegrafClass,
			},
			classes: map[string]string{testTelegrafClass: sampleClassData},
			want: want{
				Allowed: true,
				Patches: []string{
					`{"op":"add","path":"/metadata/creationTimestamp"}`,
					`{"op":"add","path":"/spec/containers/0/resources","value":{}}`,
					`{"op":"add","path":"/spec/containers/1","value":{"env":[{"name":"NODENAME","valueFrom":{"fieldRef":{"fieldPath":"spec.nodeName"}}}],"image":"docker.io/library/telegraf:1.14","name":"telegraf","resources":{"limits":{"cpu":"200m","memory":"200Mi"},"requests":{"cpu":"10m","memory":"10Mi"}},"volumeMounts":[{"mountPath":"/etc/telegraf","name":"telegraf-config"}]}}`,
					`{"op":"add","path":"/spec/containers/2","value":{"env":[{"name":"NODENAME","valueFrom":{"fieldRef":{"fieldPath":"spec.nodeName"}}}],"image":"docker.io/library/telegraf:1.14","name":"telegraf-istio","resources":{"limits":{"cpu":"200m","memory":"200Mi"},"requests":{"cpu":"10m","memory":"10Mi"}},"volumeMounts":[{"mountPath":"/etc/telegraf","name":"telegraf-istio-config"}]}}`,
					`{"op":"add","path":"/spec/volumes","value":[{"name":"telegraf-config","secret":{"secretName":"telegraf-config-simple"}},{"name":"telegraf-istio-config","secret":{"secretName":"telegraf-istio-config-simple"}}]}`,
					`{"op":"add","path":"/status","value":{}}`,
				},
			},
		},
		{
			name: "refuse to update telegraf secret if data does not match expected pattern",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Operation: admv1.Update,
					Object: runtime.RawExtension{
						Raw: []byte(`{
									"apiVersion": "v1",
									"kind": "Pod",
									"metadata": {
										"name": "simple",
									  "annotations": {
										"telegraf.influxdata.com/port": "8080",
										"telegraf.influxdata.com/path": "/v1/metrics",
										"telegraf.influxdata.com/interval": "5s"
									  }
									},
									"spec": {
									  "containers": [
										{
										  "name": "busybox",
										  "image": "busybox",
										  "args": [
											"sleep",
											"1000000"
										  ]
										}
									  ]
									}
								  }`),
					},
				},
			},
			fields: fields{
				TelegrafDefaultClass: testTelegrafClass,
			},
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "telegraf-config-simple",
					},
					Type: "Opaque",
					Data: map[string][]byte{"invalid-key": []byte(sampleClassData)},
				},
			},
			classes: map[string]string{testTelegrafClass: sampleClassData},
			want: want{
				Allowed: false,
				Patches: []string{},
				Code:    400,
				// two spaces in error message as the namespace is an empty string
				Message: "unable to update existing secret telegraf-config-simple in namespace  as it is not managed by telegraf-operator",
			},
		},
		{
			name: "refuse to update telegraf secret if type is not Opaque",
			req: admission.Request{
				AdmissionRequest: admv1.AdmissionRequest{
					Operation: admv1.Update,
					Object: runtime.RawExtension{
						Raw: []byte(`{
									"apiVersion": "v1",
									"kind": "Pod",
									"metadata": {
										"name": "simple",
									  "annotations": {
										"telegraf.influxdata.com/port": "8080",
										"telegraf.influxdata.com/path": "/v1/metrics",
										"telegraf.influxdata.com/interval": "5s"
									  }
									},
									"spec": {
									  "containers": [
										{
										  "name": "busybox",
										  "image": "busybox",
										  "args": [
											"sleep",
											"1000000"
										  ]
										}
									  ]
									}
								  }`),
					},
				},
			},
			fields: fields{
				TelegrafDefaultClass: testTelegrafClass,
			},
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "telegraf-config-simple",
					},
					Type: "Invalid",
					Data: map[string][]byte{TelegrafSecretDataKey: []byte(sampleClassData)},
				},
			},
			classes: map[string]string{testTelegrafClass: sampleClassData},
			want: want{
				Allowed: false,
				Patches: []string{},
				Code:    400,
				// two spaces in error message as the namespace is an empty string
				Message: "unable to update existing secret telegraf-config-simple in namespace  as it is not managed by telegraf-operator",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testclient.NewFakeClientWithScheme(scheme, tt.objects...)
			decoder, err := admission.NewDecoder(scheme)
			if err != nil {
				t.Fatalf("unable to create decoder: %v", err)
			}

			if tt.handler == nil {
				tt.handler = &sidecarHandler{
					RequestsCPU:    defaultRequestsCPU,
					RequestsMemory: defaultRequestsMemory,
					LimitsCPU:      defaultLimitsCPU,
					LimitsMemory:   defaultLimitsMemory,
				}
			}

			if tt.handler.TelegrafImage == "" {
				tt.handler.TelegrafImage = defaultTelegrafImage
			}

			if tt.handler.Logger == nil {
				tt.handler.Logger = &logrTesting.TestLogger{T: t}
			}

			dir := createTempClassesDirectory(t, tt.classes)
			defer os.RemoveAll(dir)

			logger := &logrTesting.TestLogger{T: t}

			testClassDataHandler := &classDataHandler{
				Logger:                   logger,
				TelegrafClassesDirectory: dir,
			}

			tt.handler.ClassDataHandler = testClassDataHandler
			tt.handler.TelegrafDefaultClass = tt.fields.TelegrafDefaultClass

			p := &podInjector{
				client:           client,
				decoder:          decoder,
				Logger:           logger,
				SidecarHandler:   tt.handler,
				ClassDataHandler: testClassDataHandler,
			}

			if tt.want.Code == 0 {
				tt.want.Code = http.StatusOK
			}

			resp := p.Handle(context.Background(), tt.req)

			if got, want := resp.Allowed, tt.want.Allowed; got != want {
				t.Errorf("podInjector.Handle().Allowed =\n%v, want\n%v", got, want)
			}

			// patches seem to come back in random order. sort to make testing easier.
			sort.Slice(resp.Patches, func(i int, j int) bool {
				return resp.Patches[i].Path < resp.Patches[j].Path
			})

			if got, want := len(resp.Patches), len(tt.want.Patches); got != want {
				t.Fatalf("invalid number of patches returned; got %d, want %d", got, want)
			}

			for i := range tt.want.Patches {
				b, err := json.Marshal(resp.Patches[i])
				if err != nil {
					t.Fatalf("unexpected error marshaling %v", err)
				}

				if got, want := string(b), tt.want.Patches[i]; got != want {
					t.Errorf("podInjector.Handle().Patches =\n%v, want\n%v", got, want)
				}
			}

			if resp.Result != nil {
				if got, want := resp.Result.Code, tt.want.Code; got != want {
					t.Errorf("podInjector.Handle().Code =\n%v, want\n%v", got, want)
				}

				if got, want := resp.Result.Message, tt.want.Message; got != want {
					t.Errorf("podInjector.Handle().Message =\n%v, want\n%v", got, want)
				}
			}
		})
	}
}

func Test_isSecretManagedByTelegrafOperator(t *testing.T) {
	testSecretData := map[string][]byte{
		TelegrafSecretDataKey: []byte("test"),
	}
	tests := []struct {
		name                        string
		secret                      *corev1.Secret
		requireAnnotationsForSecret bool
		result                      bool
	}{
		{
			name: "reports false when secret data does not match expected pattern",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					"invalid": []byte("test"),
				},
			},
			result: false,
		},
		{
			name: "reports false when secret if type is not Opaque",
			secret: &corev1.Secret{
				Type: "Invalid",
				Data: testSecretData,
			},
			result: false,
		},
		{
			name: "reports false when requireAnnotationsForSecret and annotations not present",
			secret: &corev1.Secret{
				Data: testSecretData,
			},
			requireAnnotationsForSecret: true,
			result:                      false,
		},
		{
			name: "reports false when requireAnnotationsForSecret and annotations not present",
			secret: &corev1.Secret{
				Data: testSecretData,
			},
			requireAnnotationsForSecret: true,
			result:                      false,
		},
		{
			name: "reports false when requireAnnotationsForSecret and annotations not present",
			secret: &corev1.Secret{
				Data: testSecretData,
			},
			requireAnnotationsForSecret: true,
			result:                      false,
		},
		{
			name: "reports false when requireAnnotationsForSecret and specific annotation not present",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Data: testSecretData,
			},
			requireAnnotationsForSecret: true,
			result:                      false,
		},
		{
			name: "reports false when requireAnnotationsForSecret and annotations value differs",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafSecretAnnotationKey: "somethingelse",
					},
				},
				Data: testSecretData,
			},
			requireAnnotationsForSecret: true,
			result:                      false,
		},
		{
			name: "reports true whenall conditions match",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafSecretAnnotationKey: TelegrafSecretAnnotationValue,
					},
				},
				Data: testSecretData,
			},
			requireAnnotationsForSecret: true,
			result:                      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &logrTesting.TestLogger{T: t}

			p := &podInjector{
				Logger:                      logger,
				RequireAnnotationsForSecret: tt.requireAnnotationsForSecret,
			}

			if tt.secret.TypeMeta.APIVersion == "" {
				tt.secret.TypeMeta.APIVersion = "v1"
			}
			if tt.secret.TypeMeta.Kind == "" {
				tt.secret.TypeMeta.Kind = "Secret"
			}
			if tt.secret.ObjectMeta.Namespace == "" {
				tt.secret.ObjectMeta.Namespace = "test"
			}
			if tt.secret.ObjectMeta.Name == "" {
				tt.secret.ObjectMeta.Name = "test-secret"
			}
			if tt.secret.Type == "" {
				tt.secret.Type = "Opaque"
			}

			if got, want := p.isSecretManagedByTelegrafOperator(tt.secret), tt.result; got != want {
				t.Fatalf("invalid resul; got %v, want %v", got, want)
			}
		})
	}
}
