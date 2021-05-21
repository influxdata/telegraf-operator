
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: manager

# Run tests
test: generate fmt vet manifests
	go test ./... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager *.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./*.go

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests
	kustomize build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths="./..."

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.5.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

diff:
	kubectl diff -k deploy

apply:
	kubectl apply -k deploy

genca:
	cd deploy/stubdata && openssl genrsa -out tls.key 4096 && openssl req -x509 -new -nodes -key tls.key -subj "/C=NL/ST=Zuid Holland/L=Rotterdam/O=Sparkling Network/OU=IT Department/CN=telegraf-injector.telegraf-injector.svc" -sha256 -days 1024 -out tls.crt
	kubectl delete secret telegraf-injector-certs -n telegraf-injector
	kubectl create secret tls telegraf-injector-certs -n telegraf-injector --cert=deploy/stubdata/tls.crt --key=deploy/stubdata/tls.key
	cat deploy/stubdata/tls.crt | base64 | pbcopy

kind-start:
	# create kind cluster
	kind create cluster --name=telegraf-operator-test
	# ensure correct kubectl context is set and used or fail otherwise
	kubectl config use-context kind-telegraf-operator-test
	# deploy InfluxDB in the cluster
	kubectl apply -f deploy/influxdb.yml

kind-delete:
	kind delete cluster --name=telegraf-operator-test

kind-build:
	docker build -t quay.io/influxdb/telegraf-operator:latest .
	kind load docker-image -q --name telegraf-operator-test quay.io/influxdb/telegraf-operator:latest

kind-delete-pod:
	# ensure correct kubectl context is set and used or fail otherwise
	kubectl config use-context kind-telegraf-operator-test
	kubectl delete pod --namespace=telegraf-operator -l app=telegraf-operator --wait=false

kind-cleanup:
	# ensure correct kubectl context is set and used or fail otherwise
	kubectl config use-context kind-telegraf-operator-test
	# use || true for cleanup to avoid issues when a namespace or other object does not exist
	kubectl delete MutatingWebhookConfiguration telegraf-operator	|| true
	kubectl delete namespace test || true
	kubectl delete namespace telegraf-operator || true

kind-test:
	# ensure correct kubectl context is set and used or fail otherwise
	kubectl config use-context kind-telegraf-operator-test
	kubectl apply -f examples/classes.yml
	kubectl apply -f deploy/dev.yml
	# wait 15 seconds to ensure the pod is already in running state
	sleep 15
	# deploy redis as sample deployment
	kubectl create namespace test
	kubectl apply -f examples/redis.yml
	sleep 2
	kubectl describe pod --namespace=test redis
