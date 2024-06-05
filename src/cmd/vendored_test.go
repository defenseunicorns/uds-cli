// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestAddScanCmdFlags(t *testing.T) {
	cmd := &cobra.Command{}
	addScanCmdFlags(cmd)

	tests := []struct {
		name         string
		flagName     string
		expectedType string
		expectedDef  string
	}{
		{"docker-username", "docker-username", "string", ""},
		{"docker-password", "docker-password", "string", ""},
		{"org", "org", "string", "defenseunicorns"},
		{"package-name", "package-name", "string", ""},
		{"tag", "tag", "string", ""},
		{"output-file", "output-file", "string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := cmd.PersistentFlags().Lookup(tt.flagName)
			assert.NotNil(t, flag)
			assert.Equal(t, tt.expectedType, flag.Value.Type())
			assert.Equal(t, tt.expectedDef, flag.DefValue)
		})
	}
}

func TestScanCommand(t *testing.T) {
	// Create a temporary directory for the test output file
	tempDir, err := os.MkdirTemp("", "scan-test")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir)
	outputFile := filepath.Join(tempDir, "gitlab-runner.csv")
	// Set up the command and its flags
	cmd := &cobra.Command{
		Use: "scan",
		Run: scanCmd.Run,
	}
	addScanCmdFlags(cmd)

	// Set the flags for the test
	cmd.SetArgs([]string{
		"-o", "defenseunicorns",
		"-n", "packages/uds/gitlab-runner",
		"-g", "16.10.0-uds.0-upstream",
		"-f", outputFile,
	})

	// Capture the output
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	// Run the command
	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	fileInfo, err := os.Stat(outputFile)
	if err != nil {
		t.Fatalf("failed to stat output file: %v", err)
	}
	if fileInfo.Size() == 0 {
		t.Error("output file is empty")
	}

	// Check if the output file has content
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if len(content) == 0 {
		t.Errorf("output file is empty")
	}
	t.Logf("output file size: %d", len(content))

	// Clean up
	if err := os.Remove(outputFile); err != nil {
		t.Fatalf("failed to clean up test output file: %v", err)
	}
}
