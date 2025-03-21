// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScanCommand(t *testing.T) {
	t.Log("E2E: Scan Command")

	t.Run("scan remote Zarf init pkg", func(t *testing.T) {
		t.Parallel()

		// Create a temporary directory for the test output file
		tempDir, err := os.MkdirTemp("", "scan-test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)
		outputFile := filepath.Join(tempDir, "zarf-init.csv")

		_, stdErr := runCmd(t, fmt.Sprintf("scan --org defenseunicorns --package-name packages/init --tag v0.36.1 --output-file %s", outputFile))
		require.FileExists(t, outputFile)
		fileInfo, err := os.Stat(outputFile)
		require.NoError(t, err)
		require.Greater(t, fileInfo.Size(), int64(10), "output file size should be greater than 10 bytes")
		require.NotEmpty(t, stdErr)
	})
}
