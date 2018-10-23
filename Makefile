.PHONY: all
all: generate build

PACKAGE=github.com/openshift/cluster-dns-operator
MAIN_PACKAGE=$(PACKAGE)/cmd/cluster-dns-operator

BIN=$(lastword $(subst /, ,$(MAIN_PACKAGE)))

DOCKERFILE=images/cluster-dns-operator/Dockerfile
IMAGE_TAG=openshift/origin-cluster-dns-operator

ENVVAR=GOOS=linux GOARCH=amd64 CGO_ENABLED=0
GOOS=linux
GO_BUILD_RECIPE=GOOS=$(GOOS) go build -o $(BIN) $(MAIN_PACKAGE)

.PHONY: build
build:
	$(GO_BUILD_RECIPE)

.PHONY: generate
generate:
	hack/update-generated-bindata.sh

.PHONY: test
test:	verify
	go test ./...

.PHONY: release-local
release-local:
	MANIFESTS=$(shell mktemp -d) hack/release-local.sh

.PHONY: test-integration
test-integration:
	hack/test-integration.sh

.PHONY: verify
verify:
	hack/verify-gofmt.sh
	hack/verify-generated-bindata.sh

.PHONY: local-image
local-image:
ifdef USE_BUILDAH
	@echo "  - Building with buildah ... "
	buildah bud -t $(IMAGE_TAG) -f $(DOCKERFILE) .
else
	@echo "  - Building with docker ... "
	docker build -t $(IMAGE_TAG) -f $(DOCKERFILE) .
endif

.PHONY: clean
clean:
	go clean
	rm -f $(BIN)
