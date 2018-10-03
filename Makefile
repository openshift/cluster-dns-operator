all: generate build

PACKAGE=github.com/openshift/cluster-dns-operator
MAIN_PACKAGE=$(PACKAGE)/cmd/cluster-dns-operator

BIN=$(lastword $(subst /, ,$(MAIN_PACKAGE)))
BINDATA=pkg/manifests/bindata.go
TEST_BINDATA=test/manifests/bindata.go

GOFMT_CHECK=$(shell find . -not \( \( -wholename './.*' -o -wholename '*/vendor/*' -o -wholename './pkg/assets/bindata.go' -o -wholename '././test/manifests/bindata.go' -o -wholename './pkg/manifests/bindata.go' -o -wholename './test/manifests/bindata.go' \) -prune \) -name '*.go' | sort -u | xargs gofmt -s -l)

DOCKERFILE=images/cluster-dns-operator/Dockerfile
IMAGE_TAG=openshift/origin-cluster-dns-operator

vpath bin/go-bindata $(GOPATH)
GOBINDATA_BIN=bin/go-bindata

ENVVAR=GOOS=linux GOARCH=amd64 CGO_ENABLED=0
GOOS=linux
GO_BUILD_RECIPE=GOOS=$(GOOS) go build -o $(BIN) $(MAIN_PACKAGE)

build:
	$(GO_BUILD_RECIPE)

# Using "-modtime 1" to make generate target deterministic. It sets all file time stamps to unix timestamp 1
generate: $(GOBINDATA_BIN)
	go-bindata -mode 420 -modtime 1 -pkg manifests -o $(BINDATA) manifests/... assets/...
	go-bindata -mode 420 -modtime 1 -pkg manifests -o $(TEST_BINDATA) test/assets/...

$(GOBINDATA_BIN):
	go get -u github.com/jteeuwen/go-bindata/...

test:	verify
	go test ./...

test-integration:
	go test -v -tags integration ./test/integration

verify:	verify-gofmt

verify-gofmt:
ifeq (, $(GOFMT_CHECK))
	@echo "  - verify-gofmt: OK"
else
	@echo "ERROR: gofmt failed on the following files:"
	@echo "$(GOFMT_CHECK)"
	@echo ""
	@echo "For details, run: gofmt -d -s $(GOFMT_CHECK)"
	@echo ""
	@exit -1
endif

local-image:
ifdef USE_BUILDAH
	@echo "  - Building with buildah ... "
	buildah bud -t $(IMAGE_TAG) -f $(DOCKERFILE) .
else
	@echo "  - Building with docker ... "
	docker build -t $(IMAGE_TAG) -f $(DOCKERFILE) .
endif

clean:
	go clean
	rm -f $(BIN)

.PHONY: all build generate verify verify-gofmt test test-integration clean
