.PHONY: all
all: generate build

include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
    targets/openshift/operator/profile-manifests.mk \
)

PACKAGE=github.com/openshift/cluster-dns-operator
MAIN_PACKAGE=$(PACKAGE)/cmd/dns-operator

BIN=$(lastword $(subst /, ,$(MAIN_PACKAGE)))

IMAGE_TAG=openshift/origin-cluster-dns-operator

GO=GO111MODULE=on GOFLAGS=-mod=vendor go
GO_BUILD_RECIPE=CGO_ENABLED=0 $(GO) build -o $(BIN) $(MAIN_PACKAGE)

TEST ?= .*

# This will include additional actions on the update and verify targets to ensure that profile patches are applied
# to manifest files
# $0 - macro name
# $1 - target name
# $2 - profile patches directory
# $3 - manifests directory
$(call add-profile-manifests,manifests,./profile-patches,./manifests)

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
generate: crd update

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
	hack/verify-deps.sh

.PHONY: local-image
local-image:
ifeq ($(CONTAINER_ENGINE), USE_BUILDAH)
	echo "  - Building with buildah ... "
	buildah bud -t $(IMAGE_TAG) .
else ifeq ($(CONTAINER_ENGINE), docker)
	echo "  - Building with docker ... "
	docker build -t $(IMAGE_TAG) .
else ifeq ($(CONTAINER_ENGINE), podman)
	echo "  - Building with podman ... "
	podman build -t $(IMAGE_TAG) .
else
	echo "  Please pass a container engine ... "
endif

.PHONY: run-local
run-local: build
	hack/run-local.sh

.PHONY: clean
clean:
	$(GO) clean
	rm -f $(BIN)

LOCALBIN ?= $(shell pwd)/tmp
GOVULNCHECK = $(LOCALBIN)/govulncheck

.PHONY: vulncheck
vulncheck: $(GOVULNCHECK)
	$(GOVULNCHECK) ./...

# Dependencies / Tools specifics

$(LOCALBIN):
	[ -d $@ ] || mkdir -p $@

$(GOVULNCHECK): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install golang.org/x/vuln/cmd/govulncheck@latest
