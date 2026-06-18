// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/printer"
)

func TestInspectOptions_Complete(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantBundlePath string
	}{
		{
			name:           "with file path",
			args:           []string{"path/to/bundle.uds.hcl"},
			wantBundlePath: "path/to/bundle.uds.hcl",
		},
		{
			name:           "without args defaults to current directory",
			args:           []string{},
			wantBundlePath: ".",
		},
		{
			name:           "with directory path",
			args:           []string{"path/to/dir"},
			wantBundlePath: "path/to/dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			o := NewInspectOptions(streams)

			cmd := &cobra.Command{}
			err := o.Complete(cmd, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBundlePath, o.BundlePath)
		})
	}
}

func TestInspectOptions_Validate(t *testing.T) {
	// Use the existing spec-compliant bundle for valid file test case
	existingFile := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "spec-compliant", "bundle.uds.hcl")
	existingDir := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "spec-compliant")

	// Create temporary test files and directories
	tempDir := t.TempDir()

	// Create a directory with .hcl suffix but is actually a directory
	hclSuffixDir := filepath.Join(tempDir, "bundle.uds.hcl")
	if err := os.Mkdir(hclSuffixDir, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create a directory without bundle.uds.hcl
	emptyDir := filepath.Join(tempDir, "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create a valid directory with bundle.uds.hcl
	validDir := filepath.Join(tempDir, "valid")
	if err := os.Mkdir(validDir, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	validBundleFile := filepath.Join(validDir, "bundle.uds.hcl")
	if err := os.WriteFile(validBundleFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create a tar.zst file
	tarZstFile := filepath.Join(tempDir, "bundle.tar.zst")
	if err := os.WriteFile(tarZstFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create an HCL file with wrong name
	wrongNameFile := filepath.Join(tempDir, "wrongname.hcl")
	if err := os.WriteFile(wrongNameFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tests := []struct {
		name       string
		bundlePath string
		wantErr    string
	}{
		{
			name:       "valid HCL file that exists",
			bundlePath: existingFile,
			wantErr:    "",
		},
		{
			name:       "valid directory with bundle.uds.hcl",
			bundlePath: existingDir,
			wantErr:    "",
		},
		{
			name:       "valid directory created in test",
			bundlePath: validDir,
			wantErr:    "",
		},
		{
			name:       "empty path",
			bundlePath: "",
			wantErr:    "bundle file path is required",
		},
		{
			name:       "OCI reference with scheme",
			bundlePath: "oci://ghcr.io/test/bundle:v1",
			wantErr:    "OCI bundle references not yet supported",
		},
		{
			name:       "OCI reference without scheme",
			bundlePath: "ghcr.io/test/bundle:v1",
			wantErr:    "OCI bundle references not yet supported",
		},
		{
			name:       "tar.zst archive",
			bundlePath: tarZstFile,
			wantErr:    "tar.zst bundles are not supported",
		},
		{
			name:       "tar.zst archive path that doesn't exist",
			bundlePath: "bundle.tar.zst",
			wantErr:    "tar.zst bundles are not supported",
		},
		{
			name:       "non-HCL file",
			bundlePath: "bundle.yaml",
			wantErr:    "bundle path not found",
		},
		{
			name:       "HCL file that does not exist",
			bundlePath: "/nonexistent/bundle.uds.hcl",
			wantErr:    "bundle path not found",
		},
		{
			name:       "HCL file with wrong name",
			bundlePath: wrongNameFile,
			wantErr:    "expected file named 'bundle.uds.hcl'",
		},
		{
			name:       "directory with .hcl suffix",
			bundlePath: hclSuffixDir,
			wantErr:    "directory does not contain bundle.uds.hcl",
		},
		{
			name:       "directory without bundle.uds.hcl",
			bundlePath: emptyDir,
			wantErr:    "directory does not contain bundle.uds.hcl",
		},
	}

	defaults := NewConfigResolver().Defaults()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			o := &InspectOptions{
				BundlePath: tt.bundlePath,
				Config:     &bundle.UDSBundleConfig{Global: &bundle.GlobalOptions{}, Options: &defaults},
				IOStreams:  streams,
			}

			err := o.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				// Validate should not modify BundlePath
				assert.Equal(t, tt.bundlePath, o.BundlePath, "Validate() should not modify BundlePath")
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestInspectOptions_Run(t *testing.T) {
	existingFile := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "spec-compliant", "bundle.uds.hcl")
	existingDir := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "spec-compliant")

	tests := []struct {
		name       string
		bundlePath string
		wantErr    bool
		wantOutput []string
	}{
		{
			name:       "valid bundle file",
			bundlePath: existingFile,
			wantErr:    false,
			wantOutput: []string{"uds-core-example", "core_base"},
		},
		{
			name:       "valid directory (resolved in Run)",
			bundlePath: existingDir,
			wantErr:    false,
			wantOutput: []string{"uds-core-example", "core_base"},
		},
	}

	defaults := NewConfigResolver().Defaults()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, out, _ := iostreams.NewTestIOStreams()
			textPrinter, _ := printer.NewPrinter(printer.FormatText)
			o := &InspectOptions{
				BundlePath: tt.bundlePath,
				Config:     &bundle.UDSBundleConfig{Global: &bundle.GlobalOptions{}, Options: &defaults},
				Printer:    textPrinter,
				IOStreams:  streams,
			}

			err := o.Run(context.Background())
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				// Verify that output was written
				for _, expected := range tt.wantOutput {
					assert.Contains(t, out.String(), expected)
				}
			}
		})
	}
}

func TestInspectOptions_Run_JSONOutput(t *testing.T) {
	existingFile := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "spec-compliant", "bundle.uds.hcl")

	streams, _, out, _ := iostreams.NewTestIOStreams()
	jsonPrinter, err := printer.NewPrinter(printer.FormatJSON)
	require.NoError(t, err)

	defaults := NewConfigResolver().Defaults()
	o := &InspectOptions{
		BundlePath: existingFile,
		Config:     &bundle.UDSBundleConfig{Global: &bundle.GlobalOptions{}, Options: &defaults},
		Printer:    jsonPrinter,
		IOStreams:  streams,
	}

	require.NoError(t, o.Run(context.Background()))

	// Verify stdout contains valid JSON with expected fields
	var result map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &result), "output should be valid JSON")
	assert.Equal(t, "uds-core-example", result["name"])
	packages, ok := result["packages"].([]any)
	require.True(t, ok, "packages should be an array")
	assert.NotEmpty(t, packages)
}

func TestInspectOptions_Run_YAMLOutput(t *testing.T) {
	existingFile := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "spec-compliant", "bundle.uds.hcl")

	streams, _, out, _ := iostreams.NewTestIOStreams()
	yamlPrinter, err := printer.NewPrinter(printer.FormatYAML)
	require.NoError(t, err)

	defaults := NewConfigResolver().Defaults()
	o := &InspectOptions{
		BundlePath: existingFile,
		Config:     &bundle.UDSBundleConfig{Global: &bundle.GlobalOptions{}, Options: &defaults},
		Printer:    yamlPrinter,
		IOStreams:  streams,
	}

	require.NoError(t, o.Run(context.Background()))

	// Verify stdout contains valid YAML with expected fields
	var result map[string]any
	require.NoError(t, yaml.Unmarshal(out.Bytes(), &result), "output should be valid YAML")
	assert.Equal(t, "uds-core-example", result["name"])
	packages, ok := result["packages"].([]any)
	require.True(t, ok, "packages should be an array")
	assert.NotEmpty(t, packages)
}
