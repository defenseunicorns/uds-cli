// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"strings"
	"testing"

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
