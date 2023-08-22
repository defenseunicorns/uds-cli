# SPDX-License-Identifier: Apache-2.0
# SPDX-FileCopyrightText: 2023-Present The UDS Authors

ARCH ?= amd64
CLI_VERSION ?= $(if $(shell git describe --tags),$(shell git describe --tags),"UnknownVersion")
BUILD_ARGS := -s -w -X 'github.com/defenseunicorns/uds-cli/src/config.CLIVersion=$(CLI_VERSION)'

tmp:
	echo $(CLI_VERSION)

.PHONY: help
help: ## Display this help information
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | sort | awk 'BEGIN {FS = ":.*?## "}; \
	  {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build-cli-linux-amd: ## Build the CLI for Linux AMD64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(BUILD_ARGS)" -o build/uds main.go

build-cli-linux-arm: ## Build the CLI for Linux ARM64
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(BUILD_ARGS)" -o build/uds-arm main.go

build-cli-mac-intel: ## Build the CLI for Mac Intel
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(BUILD_ARGS)" -o build/uds-mac-intel main.go

build-cli-mac-apple: ## Build the CLI for Mac Apple
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(BUILD_ARGS)" -o build/uds-mac-apple main.go

test-unit: ## Run Unit Tests
	cd src/pkg && go test ./... -failfast -v -timeout 5m

test-e2e: ## Run End to End (e2e) tests
	cd src/test/e2e && go test -failfast -v -timeout 30m

local-registry: ## Run a local docker registry
	docker run -p 5000:5000 --restart=always --name registry registry:2

clean: ## Clean up build artifacts
	rm -rf build
