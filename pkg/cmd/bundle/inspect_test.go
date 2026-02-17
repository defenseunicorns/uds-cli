// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestInspectOptions_Complete(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantBundleRef string
	}{
		{
			name:          "with file path",
			args:          []string{"path/to/bundle.uds.hcl"},
			wantBundleRef: "path/to/bundle.uds.hcl",
		},
		{
			name:          "without args defaults to current directory",
			args:          []string{},
			wantBundleRef: ".",
		},
		{
			name:          "with directory path",
			args:          []string{"path/to/dir"},
			wantBundleRef: "path/to/dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			o := NewInspectOptions(streams)

			cmd := NewInspectCommand(streams)
			err := o.Complete(cmd, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBundleRef, o.BundleRef)
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
		name             string
		bundleRef        string
		wantErr          string
		wantBundleRef    string // The expected BundleRef after validation (if different)
		expectRefChanged bool   // Whether we expect BundleRef to be modified
	}{
		{
			name:      "valid HCL file that exists",
			bundleRef: existingFile,
			wantErr:   "",
		},
		{
			name:             "valid directory with bundle.uds.hcl",
			bundleRef:        existingDir,
			wantErr:          "",
			wantBundleRef:    filepath.Join(existingDir, "bundle.uds.hcl"),
			expectRefChanged: true,
		},
		{
			name:             "valid directory created in test",
			bundleRef:        validDir,
			wantErr:          "",
			wantBundleRef:    validBundleFile,
			expectRefChanged: true,
		},
		{
			name:      "empty path",
			bundleRef: "",
			wantErr:   "bundle file path is required",
		},
		{
			name:      "OCI reference with scheme",
			bundleRef: "oci://ghcr.io/test/bundle:v1",
			wantErr:   "OCI inspection not yet supported",
		},
		{
			name:      "OCI reference without scheme",
			bundleRef: "ghcr.io/test/bundle:v1",
			wantErr:   "OCI inspection not yet supported",
		},
		{
			name:      "tar.zst archive",
			bundleRef: tarZstFile,
			wantErr:   "tar.zst inspection not yet supported",
		},
		{
			name:      "tar.zst archive path that doesn't exist",
			bundleRef: "bundle.tar.zst",
			wantErr:   "tar.zst inspection not yet supported",
		},
		{
			name:      "non-HCL file",
			bundleRef: "bundle.yaml",
			wantErr:   "bundle path not found",
		},
		{
			name:      "HCL file that does not exist",
			bundleRef: "/nonexistent/bundle.uds.hcl",
			wantErr:   "bundle path not found",
		},
		{
			name:      "HCL file with wrong name",
			bundleRef: wrongNameFile,
			wantErr:   "expected file named 'bundle.uds.hcl'",
		},
		{
			name:      "directory with .hcl suffix",
			bundleRef: hclSuffixDir,
			wantErr:   "directory does not contain bundle.uds.hcl",
		},
		{
			name:      "directory without bundle.uds.hcl",
			bundleRef: emptyDir,
			wantErr:   "directory does not contain bundle.uds.hcl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			o := &InspectOptions{
				BundleRef: tt.bundleRef,
				IOStreams: streams,
			}

			err := o.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				if tt.expectRefChanged {
					assert.Equal(t, tt.wantBundleRef, o.BundleRef, "BundleRef should be updated to point to the HCL file")
				}
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestInspectOptions_Run(t *testing.T) {
	existingFile := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "spec-compliant", "bundle.uds.hcl")

	tests := []struct {
		name      string
		bundleRef string
		wantErr   bool
	}{
		{
			name:      "valid bundle file",
			bundleRef: existingFile,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, out, _ := iostreams.NewTestIOStreams()
			o := &InspectOptions{
				BundleRef: tt.bundleRef,
				IOStreams: streams,
			}

			err := o.Run()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				// Verify that output was written
				assert.Contains(t, out.String(), "uds-core-example")
				assert.Contains(t, out.String(), "core_base")
			}
		})
	}
}
