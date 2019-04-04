.PHONY: all
all: generate build

PACKAGE=github.com/openshift/cluster-dns-operator
MAIN_PACKAGE=$(PACKAGE)/cmd/cluster-dns-operator

BIN=$(lastword $(subst /, ,$(MAIN_PACKAGE)))

IMAGE_TAG=openshift/origin-cluster-dns-operator

ENVVAR=GOOS=linux CGO_ENABLED=0
GOOS=linux
GO_BUILD_RECIPE=GOOS=$(GOOS) go build -o $(BIN) $(MAIN_PACKAGE)

.PHONY: build
build:
	$(GO_BUILD_RECIPE)

.PHONY: generate
generate: crd
	hack/update-generated-bindata.sh

# Generate CRD from vendored API spec.
.PHONY: crd
crd:
	go run ./vendor/github.com/openshift/library-go/cmd/crd-schema-gen/main.go --apis-dir vendor/github.com/openshift/api

# Do not write the CRD, only compare and return (code 1 if dirty).
.PHONY: verify-crd
verify-crd:
	go run ./vendor/github.com/openshift/library-go/cmd/crd-schema-gen/main.go --apis-dir vendor/github.com/openshift/api --verify-only

.PHONY: test
test:	verify
	go test ./...

.PHONY: release-local
release-local:
	MANIFESTS=$(shell mktemp -d) hack/release-local.sh

.PHONY: test-e2e
test-e2e:
	KUBERNETES_CONFIG="$(KUBECONFIG)" go test -v -tags e2e ./...

.PHONY: verify
verify: verify-crd
	hack/verify-gofmt.sh
	hack/verify-generated-bindata.sh

.PHONY: local-image
local-image:
ifdef USE_BUILDAH
	@echo "  - Building with buildah ... "
	buildah bud -t $(IMAGE_TAG) .
else
	@echo "  - Building with docker ... "
	docker build -t $(IMAGE_TAG) .
endif

.PHONY: clean
clean:
	go clean
	rm -f $(BIN)
