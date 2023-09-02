# SPDX-License-Identifier: Apache-2.0
# SPDX-FileCopyrightText: 2023-Present The UDS Authors

# Figure what operating system and arch we are on.
# Set some variables used to build the apporpriate binary
CGO ?= 0
UNAME_S := $(shell uname -s | tr 'A-Z' 'a-z')
UNAME_M := $(shell uname -m)
ifeq ($(UNAME_M), x86_64)
    ARCH := amd64
    ifeq ($(UNAME_S), darwin)
        OUTPUT := build/uds-mac-intel
    else ifeq ($(UNAME_S), linux)
        OUTPUT := build/uds
    else
        $(error Unsupported system: $(UNAME_S), $(UNAME_M))
	endif
else ifeq ($(UNAME_M), amd64)
    ARCH := amd64
    ifeq ($(UNAME_S), linux)
        OUTPUT := build/uds
    else
        $(error Unsupported system: $(UNAME_S), $(UNAME_M))
    endif
else ifeq ($(UNAME_M), arm64)
    ARCH := arm64
    ifeq ($(UNAME_S), linux)
        OUTPUT := build/uds-arm
    else ifeq ($(UNAME_S), darwin)
        OUTPUT := build/uds-mac-apple
    else
        $(error Unsupported system: $(UNAME_S), $(UNAME_M))
    endif
else
    $(error Unsupported architecture: $(UNAME_M))
endif

CLI_VERSION ?= $(if $(shell git describe --tags),$(shell git describe --tags),"UnknownVersion")
BUILD_ARGS := -s -w -X 'github.com/defenseunicorns/uds-cli/src/config.CLIVersion=$(CLI_VERSION)'

.PHONY: help
help: ## Display this help information
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | sort | awk 'BEGIN {FS = ":.*?## "}; \
	  {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build-cli: ## Build the CLI
	CGO_ENABLED="${CGO}" GOOS="${UNAME_S}" GOARCH="${ARCH}" go build -ldflags="$(BUILD_ARGS)" -o "${OUTPUT}" main.go

test-unit: ## Run Unit Tests
	cd src/pkg && go test ./... -failfast -v -timeout 5m

test-e2e: ## Run End to End (e2e) tests
	cd src/test/e2e && go test -failfast -v -timeout 30m

schema: ## Update JSON schema for uds-bundle.yaml
	./hack/generate-schema.sh

test-schema: ## Test if the schema has been modified
	$(MAKE) schema
	./hack/test-generate-schema.sh

local-registry: ## Run a local docker registry
	docker run -p 5000:5000 --restart=always --name registry registry:2

clean: ## Clean up build artifacts
	rm -rf build
