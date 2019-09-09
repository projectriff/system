
components := build core knative streaming

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

.PHONY: all
all: manager

# Run tests
.PHONY: test
test: generate fmt vet manifests
	go test ./... -coverprofile cover.out

# Package for distribution
.PHONY: package
package: generate fmt vet manifests
	kustomize build build/config/default > config/riff-build.yaml
	kustomize build core/config/default > config/riff-core.yaml
	kustomize build knative/config/default > config/riff-knative.yaml
	kustomize build streaming/config/default > config/riff-streaming.yaml

# Build manager binaries
.PHONY: manager
manager: generate
	$(MAKE) -C build manager
	$(MAKE) -C core manager
	$(MAKE) -C knative manager
	$(MAKE) -C streaming manager

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests:
	$(MAKE) -C build manifests
	$(MAKE) -C core manifests
	$(MAKE) -C knative manifests
	$(MAKE) -C streaming manifests

# Run go fmt against code
.PHONY: fmt
fmt:
	go fmt ./...

# Run go vet against code
.PHONY: vet
vet:
	go vet ./...

# Generate code
.PHONY: generate
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths="./..."

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.0
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif
