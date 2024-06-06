// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScanCommand(t *testing.T) {
	t.Log("E2E: Scan Command")

	t.Run("scan packages/uds/gitlab-runner", func(t *testing.T) {
		t.Parallel()

		// Create a temporary directory for the test output file
		tempDir, err := os.MkdirTemp("", "scan-test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)
		outputFile := filepath.Join(tempDir, "gitlab-runner.csv")

		stdOut, stdErr, err := e2e.UDS("scan", "--org", "defenseunicorns", "--package-name", "packages/uds/gitlab-runner", "--tag", "16.10.0-uds.0-upstream", "--output-file", outputFile)
		require.NoError(t, err, stdOut, stdErr)
		require.FileExists(t, outputFile)
		fileInfo, err := os.Stat(outputFile)
		require.NoError(t, err)
		require.Greater(t, fileInfo.Size(), int64(10), "output file size should be greater than 10 bytes")
		require.NotEmpty(t, stdOut)
		require.NotEmpty(t, stdErr)
	})
}
