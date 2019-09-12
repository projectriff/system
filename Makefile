
# Target component to build/run
COMPONENT ?= build
# Image URL to use all building/pushing image targets
IMG ?= $(COMPONENT)-controller:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= crd:trivialVersions=true

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

.PHONY: all
all: manager test

.PHONY: run
run: manifests ## Run component against the configured Kubernetes cluster in ~/.kube/config
	go run ./cmd/managers/$(COMPONENT)/main.go

.PHONY: install
install: manifests ## Install component CRDs into a cluster
	kustomize build config/$(COMPONENT)/crd | kubectl apply -f -

.PHONY: deploy
deploy: manifests ## Deploy component controller in the configured Kubernetes cluster in ~/.kube/config
	cd config/$(COMPONENT)/manager && kustomize edit set image controller=${IMG}
	kustomize build config/$(COMPONENT)/default | kubectl apply -f -

# Build component docker image
.PHONY: docker-build
docker-build: test
	docker build . --build-arg component=$(COMPONENT) -t ${IMG}

# Push a docker image
.PHONY: docker-push
docker-push:
	docker push ${IMG}

.PHONY: test
test: generate fmt vet manifests ## Run tests
	go test ./... -coverprofile cover.out

.PHONY: package
package: generate fmt vet manifests ## Package for distribution
	kustomize build config/build/default > config/riff-build.yaml
	kustomize build config/core/default > config/riff-core.yaml
	kustomize build config/knative/default > config/riff-knative.yaml
	kustomize build config/streaming/default > config/riff-streaming.yaml

# Build manager binaries
.PHONY: manager
manager: generate
	go build -o bin/build-manager ./cmd/managers/build/main.go
	go build -o bin/core-manager ./cmd/managers/core/main.go
	go build -o bin/knative-manager ./cmd/managers/knative/main.go
	go build -o bin/streaming-manager ./cmd/managers/streaming/main.go

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests:
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook \
		paths="./pkg/apis/build/...;./pkg/controllers/build/..." \
		output:crd:artifacts:config=./config/build/crd/bases \
		output:rbac:artifacts:config=./config/build/rbac \
		output:webhook:artifacts:config=./config/build/webhook
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook \
		paths="./pkg/apis/core/...;./pkg/controllers/core/..." \
		output:crd:artifacts:config=./config/core/crd/bases \
		output:rbac:artifacts:config=./config/core/rbac \
		output:webhook:artifacts:config=./config/core/webhook
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook \
		paths="./pkg/apis/knative/...;./pkg/controllers/knative/..." \
		output:crd:artifacts:config=./config/knative/crd/bases \
		output:rbac:artifacts:config=./config/knative/rbac \
		output:webhook:artifacts:config=./config/knative/webhook
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook \
		paths="./pkg/apis/streaming/...;./pkg/controllers/streaming/..." \
		output:crd:artifacts:config=./config/streaming/crd/bases \
		output:rbac:artifacts:config=./config/streaming/rbac \
		output:webhook:artifacts:config=./config/streaming/webhook

# Run go fmt against code
.PHONY: fmt
fmt:
	go fmt ./...

# Run go vet against code
.PHONY: vet
vet:
	go vet ./...

.PHONY: generate
generate: controller-gen ## Generate code
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths="./..."

# find or download controller-gen, download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.0
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

# Absolutely awesome: http://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help: ## Print help for each make target
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
