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

clean:
	rm -rf build
