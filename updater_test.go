package main

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	logrTesting "github.com/go-logr/logr/testing"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// mockSidecarHandler mocks minimal interface of sidecar handler that updater needs, rendering strings and
// returning sorted list of objects that assembleConf() method was called for.
type mockSidecarHandler struct {
	assembleConfResults []string
}

// assembleConf generates a mock result that is not a valid telegraf configuration, but namespace, name and class name separated by dot for testing purposes
func (h *mockSidecarHandler) assembleConf(pod *corev1.Pod, className string) (string, error) {
	val := fmt.Sprintf("%s.%s.%s", pod.Namespace, pod.Name, className)
	h.assembleConfResults = append(h.assembleConfResults, val)
	return val, nil
}

// get method returns sorted list of invocations that assempleConf() method was called for.
func (h *mockSidecarHandler) get() []string {
	sort.Strings(h.assembleConfResults)
	return h.assembleConfResults
}

// secretsUpdaterTest is a helper structure to test secretsUpdater with fake objects.
type secretsUpdaterTest struct {
	logger      logr.Logger
	updater     *secretsUpdater
	fakeClient  *fake.Clientset
	mockSidecar *mockSidecarHandler
	ns1         *corev1.Namespace
	pod1        *corev1.Pod
	pod2        *corev1.Pod
	secret1     *corev1.Secret
	secret2     *corev1.Secret
	secret3     *corev1.Secret
}

// newSecretsUpdaterTest creates a new instance of secretsUpdaterTest without initializing all objects.
// The createObjects() method should be called before testing to create K8s client and other objects.
func newSecretsUpdaterTest(t *testing.T, objects ...runtime.Object) *secretsUpdaterTest {
	logger := &logrTesting.TestLogger{T: t}

	ns1 := &corev1.Namespace{
		TypeMeta: v1.TypeMeta{Kind: "Namespace", APIVersion: "v1"},
		ObjectMeta: v1.ObjectMeta{
			Name: "ns1",
		},
	}

	pod1 := &corev1.Pod{
		TypeMeta: v1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: v1.ObjectMeta{
			Name:      "pod1",
			Namespace: "ns1",
			Annotations: map[string]string{
				TelegrafClass:       "test",
				TelegrafMetricsPath: "/metrics",
				TelegrafMetricsPort: "6060",
			},
		},
	}

	pod2 := &corev1.Pod{
		TypeMeta: v1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: v1.ObjectMeta{
			Name:      "pod2",
			Namespace: "ns1",
			Annotations: map[string]string{
				TelegrafClass:       "app",
				TelegrafMetricsPath: "/metrics",
				TelegrafMetricsPort: "6060",
			},
		},
	}

	secret1 := &corev1.Secret{
		TypeMeta: v1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: v1.ObjectMeta{
			Name:      "telegraf-config-pod1",
			Namespace: "ns1",
			Labels: map[string]string{
				TelegrafSecretLabelClassName: "test",
				TelegrafSecretLabelPod:       pod1.GetObjectMeta().GetName(),
			},
		},
		Data: map[string][]byte{
			TelegrafSecretDataKey: []byte("ns1.pod1.test"),
		},
	}

	secret2 := &corev1.Secret{
		TypeMeta: v1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: v1.ObjectMeta{
			Name:      "telegraf-config-pod2",
			Namespace: "ns1",
			Labels: map[string]string{
				TelegrafSecretLabelClassName: "app",
				TelegrafSecretLabelPod:       pod2.GetObjectMeta().GetName(),
			},
		},
		Data: map[string][]byte{
			TelegrafSecretDataKey: []byte("ns1.pod2.app"),
		},
	}

	secret3 := &corev1.Secret{
		TypeMeta: v1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: v1.ObjectMeta{
			Name:      "unrelated1",
			Namespace: "ns1",
			Labels:    map[string]string{},
		},
		Data: map[string][]byte{
			"key": []byte("value"),
		},
	}

	return &secretsUpdaterTest{
		logger:  logger,
		ns1:     ns1,
		pod1:    pod1,
		pod2:    pod2,
		secret1: secret1,
		secret2: secret2,
		secret3: secret3,
	}
}

// createObjects creates Kubernetes fake clientset as well as other objects that depend on it.
func (t *secretsUpdaterTest) createObjects() {
	t.fakeClient = fake.NewSimpleClientset(
		t.ns1,
		t.pod1, t.pod2,
		t.secret1, t.secret2, t.secret3,
	)

	t.mockSidecar = &mockSidecarHandler{}

	t.updater = &secretsUpdater{
		logger:       t.logger,
		clientset:    t.fakeClient,
		assembleConf: t.mockSidecar.assembleConf,
	}

}

func Test_AssembleConfForSecretsWithLabels(t *testing.T) {
	test := newSecretsUpdaterTest(t)

	test.createObjects()
	test.updater.onChange()
	// validate that assembleConf() was called for secret1 and secret2, but not secret3
	if want, got := "ns1.pod1.test;ns1.pod2.app", strings.Join(test.mockSidecar.get(), ";"); want != got {
		t.Errorf("wrong configurations assembled; want=%q; got=%q", want, got)
	}

	// assume 4 actions called on Kubernetes client - list namespaces, list secrets, get 2 pods
	if want, got := 4, len(test.fakeClient.Actions()); want != got {
		t.Errorf("wanted %d actions to be invoked, got %d", want, got)
	}
}

func Test_Secret1Updated(t *testing.T) {
	test := newSecretsUpdaterTest(t, &corev1.Secret{})
	// store the secret value to something different than what assembleConf() will return
	test.secret2.Data[TelegrafSecretDataKey] = []byte("invalid")

	test.createObjects()
	test.updater.onChange()

	// assume 5 actions called on Kubernetes client - list namespaces, list secrets, get 2 pods, update secret
	if want, got := 5, len(test.fakeClient.Actions()); want != got {
		t.Errorf("wanted %d actions to be invoked, got %d", want, got)
	}

	// verify that last action is an update of a secret
	lastAction := test.fakeClient.Actions()[4]
	if !lastAction.Matches("update", "secrets") {
		t.Errorf("last action mismatch: %v", lastAction)
	}
}
