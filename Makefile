all: build

APP_NAME=cluster-dns-operator
BIN=cluster-dns-operator
MAIN_PKG=github.com/openshift/$(APP_NAME)/cmd/$(APP_NAME)
ENVVAR=GOOS=linux GOARCH=amd64 CGO_ENABLED=0
GOOS=linux
GO_BUILD_RECIPE=GOOS=$(GOOS) go build -o $(BIN) $(MAIN_PKG)

build: $(BIN)

$(BIN):
	$(GO_BUILD_RECIPE)

.PHONY: all build
