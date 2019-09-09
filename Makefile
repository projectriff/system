
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
test: generate manifests
	go test ./... -coverprofile cover.out

# Build manager binary
.PHONY: manager
manager: generate-pkg $(components)

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: $(components)

# Generate code
.PHONY: generate
generate: generate-pkg $(components)

# Generate API code
.PHONY: generate-pkg
generate-pkg: controller-gen
	$(CONTROLLER_GEN) object paths="./pkg/..."

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.0
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

.PHONY: $(components)
$(components):
	$(MAKE) -C $@ $(MAKECMDGOALS)
