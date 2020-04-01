package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"testing"

	admv1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	logrTesting "github.com/go-logr/logr/testing"
)

const (
	sampleClassData = `
[[outputs.file]]
  files = ["stdout"]
`
)

func Test_podInjector_getClassData(t *testing.T) {
	tests := []struct {
		name       string
		objects    []runtime.Object
		secretName string
		className  string
		namespace  string
		pod        *corev1.Pod

		want    string
		wantErr bool
	}{
		{
			name:    "secret not found",
			objects: []runtime.Object{},
			pod:     &corev1.Pod{},
			wantErr: true,
		},
		{
			name: "secret not found in other namespace",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "name",
						Namespace: "namespace",
					},
					Data: map[string][]byte{TelegrafClass: []byte(sampleClassData)},
				},
			},
			secretName: "name",
			namespace:  "not_namespace",
			pod:        &corev1.Pod{},
			wantErr:    true,
		},
		{
			name: "secret not found with different name",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "name",
						Namespace: "namespace",
					},
					Data: map[string][]byte{TelegrafClass: []byte(sampleClassData)},
				},
			},
			secretName: "not_name",
			namespace:  "namespace",
			pod:        &corev1.Pod{},
			wantErr:    true,
		},
		{
			name:      "data does not contain class name",
			className: "unknown",
			objects: []runtime.Object{
				&corev1.Secret{
					Data: map[string][]byte{TelegrafClass: []byte(sampleClassData)},
				},
			},
			pod:     &corev1.Pod{},
			wantErr: true,
		},
		{
			name:      "returns secret data",
			className: TelegrafClass,
			objects: []runtime.Object{
				&corev1.Secret{
					Data: map[string][]byte{TelegrafClass: []byte(sampleClassData)},
				},
			},
			pod:  &corev1.Pod{},
			want: sampleClassData,
		},
		{
			name:      "returns secret data with name and namespace",
			className: TelegrafClass,
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "name",
						Namespace: "namespace",
					},
					Data: map[string][]byte{TelegrafClass: []byte(sampleClassData)},
				},
			},
			secretName: "name",
			namespace:  "namespace",
			pod:        &corev1.Pod{},
			want:       sampleClassData,
		},
		{
			name: "returns secret data with annotation override",
			objects: []runtime.Object{
				&corev1.Secret{
					Data: map[string][]byte{"name_override": []byte(sampleClassData)},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						TelegrafClass: "name_override",
					},
				},
			},
			want: sampleClassData,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testclient.NewFakeClientWithScheme(scheme, tt.objects...)
			p := &podInjector{
				client:                    client,
				TelegrafClassesSecretName: tt.secretName,
				TelegrafDefaultClass:      tt.className,
				ControllerNamespace:       tt.namespace,
				Logger:                    &logrTesting.TestLogger{T: t},
				SidecarHandler: &sidecarHandler{
					TelegrafImage: defaultTelegrafImage,
				},
			}
			got, err := p.getClassData(tt.pod)
			if (err != nil) != tt.wantErr {
				t.Errorf("podInjector.getClassData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("podInjector.getClassData() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_podInjector_Handle(t *testing.T) {
	type want struct {
		Patches []string
		Allowed bool
		Code    int32
		Message string
	}
	type fields struct {
		TelegrafClassesSecretName string
		TelegrafDefaultClass      string
	}
	tests := []struct {
		name    string
		fields  fields
		objects []runtime.Object
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
				TelegrafDefaultClass: TelegrafClass,
			},
			objects: []runtime.Object{
				&corev1.Secret{
					Data: map[string][]byte{TelegrafClass: []byte(sampleClassData)},
				},
			},
			want: want{
				Allowed: true,
				Patches: []string{
					`{"op":"add","path":"/metadata/creationTimestamp"}`,
					`{"op":"add","path":"/spec/containers/0/resources","value":{}}`,
					`{"op":"add","path":"/spec/containers/1","value":{"env":[{"name":"NODENAME","valueFrom":{"fieldRef":{"fieldPath":"spec.nodeName"}}}],"image":"docker.io/library/telegraf:1.13","name":"telegraf","resources":{"limits":{"cpu":"500m","memory":"500Mi"},"requests":{"cpu":"50m","memory":"50Mi"}},"volumeMounts":[{"mountPath":"/etc/telegraf","name":"telegraf-config"}]}}`,
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
				TelegrafImage: "docker.io/library/telegraf:1.11",
			},
			fields: fields{
				TelegrafDefaultClass: TelegrafClass,
			},
			objects: []runtime.Object{
				&corev1.Secret{
					Data: map[string][]byte{TelegrafClass: []byte(sampleClassData)},
				},
			},
			want: want{
				Allowed: true,
				Patches: []string{
					`{"op":"add","path":"/metadata/creationTimestamp"}`,
					`{"op":"add","path":"/spec/containers/0/resources","value":{}}`,
					`{"op":"add","path":"/spec/containers/1","value":{"env":[{"name":"NODENAME","valueFrom":{"fieldRef":{"fieldPath":"spec.nodeName"}}}],"image":"docker.io/library/telegraf:1.11","name":"telegraf","resources":{"limits":{"cpu":"500m","memory":"500Mi"},"requests":{"cpu":"50m","memory":"50Mi"}},"volumeMounts":[{"mountPath":"/etc/telegraf","name":"telegraf-config"}]}}`,
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
				TelegrafDefaultClass: TelegrafClass,
			},
			objects: []runtime.Object{
				&corev1.Secret{
					Data: map[string][]byte{TelegrafClass: []byte(sampleClassData)},
				},
			},
			want: want{
				Allowed: true,
				Patches: []string{
					`{"op":"add","path":"/metadata/creationTimestamp"}`,
					`{"op":"add","path":"/spec/containers/0/resources","value":{}}`,
					`{"op":"add","path":"/spec/containers/1","value":{"env":[{"name":"NODENAME","valueFrom":{"fieldRef":{"fieldPath":"spec.nodeName"}}}],"image":"docker.io/library/telegraf:1.11","name":"telegraf","resources":{"limits":{"cpu":"500m","memory":"500Mi"},"requests":{"cpu":"50m","memory":"50Mi"}},"volumeMounts":[{"mountPath":"/etc/telegraf","name":"telegraf-config"}]}}`,
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
				TelegrafDefaultClass:      TelegrafClass,
				TelegrafClassesSecretName: "telegraf-config-simple",
			},
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "telegraf-config-simple",
					},
					Data: map[string][]byte{TelegrafClass: []byte(sampleClassData)},
				},
			},
			want: want{
				Allowed: true,
				Patches: []string{
					`{"op":"add","path":"/metadata/creationTimestamp"}`,
					`{"op":"add","path":"/spec/containers/0/resources","value":{}}`,
					`{"op":"add","path":"/spec/containers/1","value":{"env":[{"name":"NODENAME","valueFrom":{"fieldRef":{"fieldPath":"spec.nodeName"}}}],"image":"docker.io/library/telegraf:1.13","name":"telegraf","resources":{"limits":{"cpu":"500m","memory":"500Mi"},"requests":{"cpu":"50m","memory":"50Mi"}},"volumeMounts":[{"mountPath":"/etc/telegraf","name":"telegraf-config"}]}}`,
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
				TelegrafDefaultClass:      TelegrafClass,
				TelegrafClassesSecretName: "telegraf-config-simple",
			},
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "telegraf-config-simple",
					},
					Data: map[string][]byte{TelegrafClass: []byte(sampleClassData)},
				},
			},
			want: want{
				Code:    http.StatusOK,
				Allowed: true,
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
				tt.handler = &sidecarHandler{}
			}

			if tt.handler.TelegrafImage == "" {
				tt.handler.TelegrafImage = defaultTelegrafImage
			}

			p := &podInjector{
				client:               client,
				decoder:              decoder,
				TelegrafDefaultClass: tt.fields.TelegrafDefaultClass,
				Logger:               &logrTesting.TestLogger{T: t},
				SidecarHandler:       tt.handler,
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
