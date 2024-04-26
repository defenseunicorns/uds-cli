// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"strings"
	"testing"

	"github.com/defenseunicorns/zarf/src/pkg/utils/exec"
	"github.com/stretchr/testify/require"
)

// NOTE: These tests test that the embedded `zarf` commands are imported properly and function as expected

// TestZarfLint tests to ensure that the `zarf dev lint` command functions (which requires the zarf schema to be embedded in main.go)
func TestZarfLint(t *testing.T) {
	cmd := strings.Split("zarf dev lint src/test/packages/podinfo", " ")
	_, stdErr, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	require.Contains(t, stdErr, "Image not pinned with digest - ghcr.io/stefanprodan/podinfo:6.4.0")
}

// K9S_VERSION=$(shell go list -f '{{.Version}}' -m github.com/derailed/k9s)
// CRANE_VERSION=$(shell go list -f '{{.Version}}' -m github.com/google/go-containerregistry)
// SYFT_VERSION=$(shell go list -f '{{.Version}}' -m github.com/anchore/syft)
// ARCHIVER_VERSION=$(shell go list -f '{{.Version}}' -m github.com/mholt/archiver/v3)
// HELM_VERSION=$(shell go list -f '{{.Version}}' -m helm.sh/helm/v3)

// // vendored tool versions are set as build args
func TestZarfToolsVersions(t *testing.T) {
	cmd := strings.Split("zarf tools helm version", " ")
	_, stderr, err := e2e.UDS(cmd...)
	getHelmVersionCmd := strings.Split("list -f '{{.Version}}' -m helm.sh/helm/v3", " ")
	versionRes, _, _ := exec.Cmd("go", getHelmVersionCmd...)
	helmVersion := strings.Split(versionRes, "'")
	version := strings.Split(stderr, "\n")
	require.NoError(t, err)
	require.Contains(t, version[4], "helm")
	require.Contains(t, version[4], helmVersion[1])
}
