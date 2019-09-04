
components := build core knative streaming

.PHONY: all
all: manager

# Run tests
.PHONY: test
test: generate manifests
	go test ./... -coverprofile cover.out

# Build manager binary
.PHONY: manager
manager: $(components)

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: $(components)

# Generate code
.PHONY: generate
generate: $(components)

.PHONY: $(components)
$(components):
	$(MAKE) -C $@ $(MAKECMDGOALS)
