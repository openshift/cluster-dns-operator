.PHONY: all
all: generate build

PACKAGE=github.com/openshift/cluster-dns-operator
MAIN_PACKAGE=$(PACKAGE)/cmd/dns-operator

BIN=$(lastword $(subst /, ,$(MAIN_PACKAGE)))

IMAGE_TAG=openshift/origin-cluster-dns-operator

GO=GO111MODULE=on GOFLAGS=-mod=vendor go
GO_BUILD_RECIPE=CGO_ENABLED=0 $(GO) build -o $(BIN) $(MAIN_PACKAGE)

TEST ?= .*

.PHONY: build
build:
	$(GO_BUILD_RECIPE)

.PHONY: buildconfig
buildconfig:
	hack/create-buildconfig.sh

.PHONY: cluster-build
cluster-build:
	hack/start-build.sh

.PHONY: generate
generate: bindata crd

.PHONY: bindata
bindata:
	hack/update-generated-bindata.sh

.PHONY: crd
crd:
	hack/update-generated-crd.sh

.PHONY: test
test:
	$(GO) test ./...

.PHONY: release-local
release-local:
	MANIFESTS=$(shell mktemp -d) hack/release-local.sh

.PHONY: test-e2e
test-e2e:
	$(GO) test -timeout 1h -count 1 -v -tags e2e -run "$(TEST)" ./test/e2e

.PHONY: verify
verify:
	hack/verify-gofmt.sh
	hack/verify-generated-crd.sh
	hack/verify-generated-bindata.sh
	hack/verify-deps.sh

.PHONY: local-image
local-image:
ifdef USE_BUILDAH
	@echo "  - Building with buildah ... "
	buildah bud -t $(IMAGE_TAG) .
else
	@echo "  - Building with docker ... "
	docker build -t $(IMAGE_TAG) .
endif

.PHONY: run-local
run-local: build
	hack/run-local.sh

.PHONY: clean
clean:
	$(GO) clean
	rm -f $(BIN)
