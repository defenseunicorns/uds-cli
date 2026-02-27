// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsOCIReference(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// OCI references - should return true
		{
			name:  "OCI reference with oci:// scheme",
			input: "oci://ghcr.io/defenseunicorns/test:v1",
			want:  true,
		},
		{
			name:  "https:// scheme is not OCI",
			input: "https://ghcr.io/defenseunicorns/test:v1",
			want:  false,
		},
		{
			name:  "OCI reference without scheme",
			input: "ghcr.io/defenseunicorns/test:v1",
			want:  true,
		},
		{
			name:  "OCI reference with tag",
			input: "registry.example.com/org/repo:tag",
			want:  true,
		},
		{
			name:  "OCI reference with digest",
			input: "ghcr.io/org/repo@sha256:abc123",
			want:  true,
		},
		{
			name:  "DockerHub style reference",
			input: "docker.io/library/nginx:latest",
			want:  true,
		},
		// Local file paths - should return false
		{
			name:  "absolute path",
			input: "/path/to/bundle.uds.hcl",
			want:  false,
		},
		{
			name:  "relative path with ./",
			input: "./bundle.uds.hcl",
			want:  false,
		},
		{
			name:  "relative path with ../",
			input: "../bundle.uds.hcl",
			want:  false,
		},
		{
			name:  "Windows path with backslash",
			input: "C:\\path\\to\\bundle.uds.hcl",
			want:  false,
		},
		{
			name:  "bundle.uds.hcl filename",
			input: "bundle.uds.hcl",
			want:  false,
		},
		{
			name:  "file with .hcl extension",
			input: "my-bundle.hcl",
			want:  false,
		},
		{
			name:  "tar.zst file",
			input: "bundle.tar.zst",
			want:  false,
		},
		{
			name:  "yaml file",
			input: "config.yaml",
			want:  false,
		},
		{
			name:  "yml file",
			input: "config.yml",
			want:  false,
		},
		{
			name:  "string with spaces",
			input: "not an oci reference",
			want:  false,
		},
		{
			name:  "simple directory name",
			input: "my-bundle",
			want:  false,
		},
		{
			name:  "current directory",
			input: ".",
			want:  false,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bundle.IsOCIReference(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsTarZst(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "tar.zst file",
			input: "bundle.tar.zst",
			want:  true,
		},
		{
			name:  "tar.zst file with path",
			input: "/path/to/bundle.tar.zst",
			want:  true,
		},
		{
			name:  "tar.zst file with relative path",
			input: "./bundle.tar.zst",
			want:  true,
		},
		{
			name:  "not a tar.zst file",
			input: "bundle.uds.hcl",
			want:  false,
		},
		{
			name:  "tar file without zst",
			input: "bundle.tar",
			want:  false,
		},
		{
			name:  "zst file without tar",
			input: "bundle.zst",
			want:  false,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "tar.zst in middle of string",
			input: "bundle.tar.zst.backup",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTarZst(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateBundlePath(t *testing.T) {
	// Set up temporary test files and directories
	tempDir := t.TempDir()

	// Create a valid directory with bundle.uds.hcl
	validDir := filepath.Join(tempDir, "valid")
	require.NoError(t, os.Mkdir(validDir, 0o755))
	validBundleFile := filepath.Join(validDir, BundleFileName)
	require.NoError(t, os.WriteFile(validBundleFile, []byte("test content"), 0o644))

	// Create an empty directory (no bundle.uds.hcl)
	emptyDir := filepath.Join(tempDir, "empty")
	require.NoError(t, os.Mkdir(emptyDir, 0o755))

	// Create a directory with .hcl suffix
	hclSuffixDir := filepath.Join(tempDir, "bundle.uds.hcl")
	require.NoError(t, os.Mkdir(hclSuffixDir, 0o755))

	// Create a tar.zst file
	tarZstFile := filepath.Join(tempDir, "bundle.tar.zst")
	require.NoError(t, os.WriteFile(tarZstFile, []byte("test"), 0o644))

	// Create an HCL file with wrong name
	wrongNameFile := filepath.Join(tempDir, "wrongname.hcl")
	require.NoError(t, os.WriteFile(wrongNameFile, []byte("test"), 0o644))

	tests := []struct {
		name    string
		ref     string
		wantErr string
	}{
		// Success cases
		{
			name:    "valid HCL file",
			ref:     validBundleFile,
			wantErr: "",
		},
		{
			name:    "valid directory with bundle.uds.hcl",
			ref:     validDir,
			wantErr: "",
		},
		// Error cases - empty/invalid input
		{
			name:    "empty string",
			ref:     "",
			wantErr: "bundle file path is required",
		},
		// Error cases - OCI references (not yet supported)
		{
			name:    "OCI reference with scheme",
			ref:     "oci://ghcr.io/test/bundle:v1",
			wantErr: "OCI bundle references not yet supported",
		},
		{
			name:    "OCI reference without scheme",
			ref:     "ghcr.io/test/bundle:v1",
			wantErr: "OCI bundle references not yet supported",
		},
		// Error cases - tar.zst (not yet supported)
		{
			name:    "tar.zst archive that exists",
			ref:     tarZstFile,
			wantErr: "tar.zst bundles not yet supported",
		},
		{
			name:    "tar.zst archive path that doesn't exist",
			ref:     "nonexistent.tar.zst",
			wantErr: "tar.zst bundles not yet supported",
		},
		// Error cases - file not found
		{
			name:    "non-existent path",
			ref:     "/nonexistent/path/bundle.uds.hcl",
			wantErr: "bundle path not found",
		},
		// Error cases - directory issues
		{
			name:    "directory without bundle.uds.hcl",
			ref:     emptyDir,
			wantErr: "directory does not contain bundle.uds.hcl",
		},
		{
			name:    "directory with .hcl suffix but no bundle file",
			ref:     hclSuffixDir,
			wantErr: "directory does not contain bundle.uds.hcl",
		},
		// Error cases - wrong file name
		{
			name:    "HCL file with wrong name",
			ref:     wrongNameFile,
			wantErr: "expected file named 'bundle.uds.hcl'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBundlePath(tt.ref)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestResolveBundlePath(t *testing.T) {
	// Set up temporary test files and directories
	tempDir := t.TempDir()

	// Create a valid directory with bundle.uds.hcl
	validDir := filepath.Join(tempDir, "valid")
	require.NoError(t, os.Mkdir(validDir, 0o755))
	validBundleFile := filepath.Join(validDir, BundleFileName)
	require.NoError(t, os.WriteFile(validBundleFile, []byte("test content"), 0o644))

	tests := []struct {
		name     string
		ref      string
		wantPath string
	}{
		{
			name:     "file path returns as-is",
			ref:      validBundleFile,
			wantPath: validBundleFile,
		},
		{
			name:     "directory path resolves to bundle file",
			ref:      validDir,
			wantPath: validBundleFile,
		},
		{
			name:     "non-existent path returns as-is",
			ref:      "/nonexistent/path",
			wantPath: "/nonexistent/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveBundlePath(tt.ref)
			assert.Equal(t, tt.wantPath, got)
		})
	}
}

func TestValidateBundlePath_WithRealBundle(t *testing.T) {
	// Test with the actual spec-compliant bundle from test data
	bundleDir := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "spec-compliant")
	bundleFile := filepath.Join(bundleDir, BundleFileName)

	t.Run("validate directory", func(t *testing.T) {
		err := ValidateBundlePath(bundleDir)
		require.NoError(t, err)
	})

	t.Run("validate file", func(t *testing.T) {
		err := ValidateBundlePath(bundleFile)
		require.NoError(t, err)
	})
}

func TestResolveBundlePath_WithRealBundle(t *testing.T) {
	// Test with the actual spec-compliant bundle from test data
	bundleDir := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "spec-compliant")
	bundleFile := filepath.Join(bundleDir, BundleFileName)

	t.Run("resolve directory to bundle file", func(t *testing.T) {
		got := ResolveBundlePath(bundleDir)
		assert.Equal(t, bundleFile, got)
	})

	t.Run("resolve file directly", func(t *testing.T) {
		got := ResolveBundlePath(bundleFile)
		assert.Equal(t, bundleFile, got)
	})
}
