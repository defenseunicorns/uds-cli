// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

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
			got := isOCIReference(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateDir(t *testing.T) {
	t.Run("valid directory", func(t *testing.T) {
		require.NoError(t, ValidateDir(t.TempDir()))
	})

	t.Run("empty path", func(t *testing.T) {
		require.NoError(t, ValidateDir(""))
	})

	t.Run("nonexistent path", func(t *testing.T) {
		err := ValidateDir("/nonexistent/path/tmp")
		require.ErrorIs(t, err, ErrPathNotFound)
		require.ErrorIs(t, err, os.ErrNotExist)
		require.ErrorContains(t, err, "directory does not exist")
	})

	t.Run("path is a file", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "afile")
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
		err := ValidateDir(f)
		require.ErrorContains(t, err, "path is not a directory")
	})
}

func TestValidateBundlePath(t *testing.T) {
	// Set up temporary test files and directories
	tempDir := t.TempDir()

	// Create a valid directory with bundle.uds.hcl
	validDir := filepath.Join(tempDir, "valid")
	require.NoError(t, os.Mkdir(validDir, 0o755))
	validBundleFile := filepath.Join(validDir, bundleFileName)
	require.NoError(t, os.WriteFile(validBundleFile, []byte("test content"), 0o600))

	// Create an empty directory (no bundle.uds.hcl)
	emptyDir := filepath.Join(tempDir, "empty")
	require.NoError(t, os.Mkdir(emptyDir, 0o755))

	// Create a directory with .hcl suffix
	hclSuffixDir := filepath.Join(tempDir, "bundle.uds.hcl")
	require.NoError(t, os.Mkdir(hclSuffixDir, 0o755))

	// Create a tar.zst file
	tarZstFile := filepath.Join(tempDir, "bundle.tar.zst")
	require.NoError(t, os.WriteFile(tarZstFile, []byte("test"), 0o600))

	// Create an HCL file with wrong name
	wrongNameFile := filepath.Join(tempDir, "wrongname.hcl")
	require.NoError(t, os.WriteFile(wrongNameFile, []byte("test"), 0o600))

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
			wantErr: ErrOCINotSupported.Error(),
		},
		{
			name:    "OCI reference without scheme",
			ref:     "ghcr.io/test/bundle:v1",
			wantErr: ErrOCINotSupported.Error(),
		},
		// Error cases - tar.zst (are not supported by ValidateBundlePath)
		{
			name:    "tar.zst archive that exists",
			ref:     tarZstFile,
			wantErr: "tar.zst bundles are not supported",
		},
		{
			name:    "tar.zst archive path that doesn't exist",
			ref:     "nonexistent.tar.zst",
			wantErr: "tar.zst bundles are not supported",
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
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateBundlePathClassifiesAccessFailure(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(parentFile, nil, 0o600))

	err := ValidateBundlePath(filepath.Join(parentFile, bundleFileName))
	require.ErrorIs(t, err, ErrInvalidPath)
	require.NotErrorIs(t, err, ErrPathNotFound)
	var pathErr *fs.PathError
	require.ErrorAs(t, err, &pathErr)
}

func TestValidateBundlePath_AllowArtifact(t *testing.T) {
	tempDir := t.TempDir()

	tarZstFile := filepath.Join(tempDir, "bundle.tar.zst")
	require.NoError(t, os.WriteFile(tarZstFile, []byte("test"), 0o600))

	validDir := filepath.Join(tempDir, "valid")
	require.NoError(t, os.Mkdir(validDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(validDir, bundleFileName), []byte(""), 0o600))

	tests := []struct {
		name    string
		ref     string
		wantErr string
	}{
		{name: "tar.zst that exists", ref: tarZstFile, wantErr: ""},
		{name: "tar.zst that does not exist", ref: "nonexistent.tar.zst", wantErr: "bundle artifact not found"},
		{name: "valid directory", ref: validDir, wantErr: ""},
		{name: "OCI reference rejected", ref: "oci://ghcr.io/test/bundle:v1", wantErr: ErrOCINotSupported.Error()},
		{name: "empty string", ref: "", wantErr: "bundle file path is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBundlePath(tt.ref, AllowArtifactBundlePath())
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateBundlePathPrefersOCIReferenceOverTarZstSuffix(t *testing.T) {
	tests := []string{
		"oci://ghcr.io/test/bundle:v1.tar.zst",
		"ghcr.io/test/bundle:v1.tar.zst",
	}
	for _, ref := range tests {
		t.Run(ref, func(t *testing.T) {
			err := ValidateBundlePath(ref, AllowArtifactBundlePath(), AllowOCIReferenceBundlePath())
			require.NoError(t, err)
		})
	}
}

func TestValidateBundlePath_WithRealBundle(t *testing.T) {
	// Test with the actual spec-compliant bundle from test data
	bundleDir := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "spec-compliant")
	bundleFile := filepath.Join(bundleDir, bundleFileName)

	t.Run("validate directory", func(t *testing.T) {
		err := ValidateBundlePath(bundleDir)
		require.NoError(t, err)
	})

	t.Run("validate file", func(t *testing.T) {
		err := ValidateBundlePath(bundleFile)
		require.NoError(t, err)
	})
}

func TestValidateDevDeployPath_RedirectsArtifacts(t *testing.T) {
	for _, ref := range []string{"bundle.tar.zst", "oci://ghcr.io/example/bundle:1.0.0", "ghcr.io/example/bundle:1.0.0"} {
		err := ValidateDevDeployPath(ref)
		require.ErrorContains(t, err, "uds bundle deploy")
	}
}

func TestValidateArtifactReference_RedirectsSource(t *testing.T) {
	dir := t.TempDir()
	bundleFile := filepath.Join(dir, bundleFileName)
	require.NoError(t, os.WriteFile(bundleFile, []byte("test"), 0o600))

	for _, ref := range []string{dir, bundleFile} {
		err := ValidateArtifactReference(ref)
		require.ErrorContains(t, err, "uds bundle dev deploy")
	}
}

func TestDeployValidators_PreferExistingRelativeSourcePaths(t *testing.T) {
	t.Chdir(t.TempDir())

	for _, dir := range []string{filepath.Join("example.com", "bundle"), "source.tar.zst"} {
		require.NoError(t, os.MkdirAll(dir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, bundleFileName), []byte("test"), 0o600))

		require.NoError(t, ValidateDevDeployPath(dir))
		err := ValidateArtifactReference(dir)
		require.ErrorContains(t, err, "uds bundle dev deploy")
	}
}

func TestDeployValidators_DoNotTreatInaccessibleLocalPathAsOCI(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.Mkdir("example.com", 0o700))
	require.NoError(t, os.Symlink("bundle", filepath.Join("example.com", "bundle")))

	for _, validate := range []func(string) error{ValidateDevDeployPath, ValidateArtifactReference} {
		err := validate(filepath.Join("example.com", "bundle"))
		require.ErrorContains(t, err, "cannot access")
	}
}
