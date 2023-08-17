# SPDX-License-Identifier: Apache-2.0
# SPDX-FileCopyrightText: 2023-Present The UDS Authors

ARCH ?= amd64
BUILD_ARGS := -s -w # remove debugging info

build-cli-linux-amd:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(BUILD_ARGS)" -o build/uds main.go

build-cli-linux-arm:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(BUILD_ARGS)" -o build/uds-arm main.go

build-cli-mac-intel:
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(BUILD_ARGS)" -o build/uds-mac-intel main.go

build-cli-mac-apple:
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(BUILD_ARGS)" -o build/uds-mac-apple main.go

test-unit:
	cd src/pkg && go test ./... -failfast -v -timeout 5m

test-e2e:
	cd src/test/e2e && go test -failfast -v -timeout 30m

local-registry:
	docker run -p 5000:5000 --restart=always --name registry registry:2

clean:
	rm -rf build
