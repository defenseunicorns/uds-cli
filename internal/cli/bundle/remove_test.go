// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestRemoveOptions_Complete(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantBundlePath string
	}{
		{
			name:           "with artifact path",
			args:           []string{"path/to/bundle.tar.zst"},
			wantBundlePath: "path/to/bundle.tar.zst",
		},
		{
			name:           "with OCI artifact",
			args:           []string{"oci://example.com/bundle:v1"},
			wantBundlePath: "oci://example.com/bundle:v1",
		},
		{
			name:           "without args defaults to current directory",
			args:           []string{},
			wantBundlePath: ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			o := NewRemoveOptions(streams)

			bundleCmd := NewBundleCommand(streams)
			cmd, _, _ := bundleCmd.Find([]string{"remove"})
			err := o.Complete(cmd, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBundlePath, o.BundlePath)
		})
	}
}

func TestDevRemoveOptions_Complete(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantBundlePath string
	}{
		{
			name:           "with HCL file path",
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
			o := NewDevRemoveOptions(streams)

			bundleCmd := NewBundleCommand(streams)
			cmd, _, _ := bundleCmd.Find([]string{"dev", "remove"})
			err := o.Complete(cmd, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBundlePath, o.BundlePath)
		})
	}
}

func TestRemoveOptions_Validate(t *testing.T) {
	tempDir := t.TempDir()
	tarZstFile := filepath.Join(tempDir, "bundle.tar.zst")
	require.NoError(t, os.WriteFile(tarZstFile, []byte("test"), 0o600))

	existingHCLFile := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", bundleFileName)
	existingDir := filepath.Dir(existingHCLFile)
	defaults := NewConfigResolver().Defaults()

	tests := []struct {
		name       string
		bundlePath string
		wantErr    string
	}{
		{
			name:       "valid OCI artifact",
			bundlePath: "oci://example.com/bundle:v1",
		},
		{
			name:       "valid tar.zst artifact",
			bundlePath: tarZstFile,
		},
		{
			name:    "empty path",
			wantErr: "bundle file path is required",
		},
		{
			name:       "empty OCI reference",
			bundlePath: "oci://",
			wantErr:    "invalid reference: missing registry or repository",
		},
		{
			name:       "tar.zst artifact that does not exist",
			bundlePath: filepath.Join(tempDir, "missing.tar.zst"),
			wantErr:    "bundle artifact not found",
		},
		{
			name:       "HCL file",
			bundlePath: existingHCLFile,
			wantErr:    "not a valid oci reference or .tar.zst",
		},
		{
			name:       "directory",
			bundlePath: existingDir,
			wantErr:    "not a valid oci reference or .tar.zst",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			o := &RemoveOptions{
				BundlePath:   tt.bundlePath,
				Config:       &bundle.UDSBundleConfig{Options: &defaults},
				IOStreams:    streams,
				Verification: VerifyOptions{SkipSignatureVerification: true},
			}

			err := o.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestDevRemoveOptions_Validate(t *testing.T) {
	existingHCLFile := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", bundleFileName)
	existingDir := filepath.Dir(existingHCLFile)

	tempDir := t.TempDir()
	emptyDir := filepath.Join(tempDir, "empty")
	require.NoError(t, os.Mkdir(emptyDir, 0o755))
	tarZstFile := filepath.Join(tempDir, "bundle.tar.zst")
	require.NoError(t, os.WriteFile(tarZstFile, []byte("test"), 0o600))

	defaults := NewConfigResolver().Defaults()
	tests := []struct {
		name       string
		bundlePath string
		wantErr    string
	}{
		{
			name:       "valid HCL file",
			bundlePath: existingHCLFile,
		},
		{
			name:       "valid directory",
			bundlePath: existingDir,
		},
		{
			name:    "empty path",
			wantErr: "bundle file path is required",
		},
		{
			name:       "HCL file that does not exist",
			bundlePath: filepath.Join(tempDir, bundleFileName),
			wantErr:    "bundle path not found",
		},
		{
			name:       "directory without bundle.uds.hcl",
			bundlePath: emptyDir,
			wantErr:    "directory does not contain bundle.uds.hcl",
		},
		{
			name:       "OCI artifact",
			bundlePath: "oci://example.com/bundle:v1",
			wantErr:    "not a valid hcl file or directory",
		},
		{
			name:       "tar.zst artifact",
			bundlePath: tarZstFile,
			wantErr:    "not a valid hcl file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			o := &DevRemoveOptions{
				BundlePath: tt.bundlePath,
				Config:     &bundle.UDSBundleConfig{Options: &defaults},
				IOStreams:  streams,
			}

			err := o.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestRemoveOptions_Run_PromptDecline(t *testing.T) {
	artifact := createTestArtifact(t)
	defaults := NewConfigResolver().Defaults()

	tests := []struct {
		name          string
		input         string
		wantErrOutput []string
	}{
		{
			name:  "prompt flag - user confirms no",
			input: "n\n",
			wantErrOutput: []string{
				"Remove this bundle?",
			},
		},
		{
			name:          "prompt flag - empty input treated as no",
			input:         "",
			wantErrOutput: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, in, out, errOut := iostreams.NewTestIOStreams()
			in.WriteString(tt.input)

			textPrinter, _ := printer.NewPrinter(printer.FormatText)

			o := &RemoveOptions{
				BundlePath:   artifact,
				Prompt:       true,
				Config:       &bundle.UDSBundleConfig{Options: &defaults},
				Printer:      textPrinter,
				IOStreams:    streams,
				Verification: VerifyOptions{SkipSignatureVerification: true},
			}

			require.NoError(t, o.Validate())
			err := o.Run(t.Context())
			require.NoError(t, err)
			assert.Empty(t, out.String(), "stdout should be empty when removal is cancelled")
			for _, expected := range tt.wantErrOutput {
				assert.Contains(t, errOut.String(), expected)
			}
		})
	}
}

func TestDevRemoveOptions_Run_PromptDecline(t *testing.T) {
	existingHCLFile := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", bundleFileName)
	defaults := NewConfigResolver().Defaults()

	tests := []struct {
		name          string
		input         string
		wantErrOutput []string
	}{
		{
			name:  "prompt flag - user confirms no",
			input: "n\n",
			wantErrOutput: []string{
				"Remove this bundle?",
			},
		},
		{
			name:          "prompt flag - empty input treated as no",
			input:         "",
			wantErrOutput: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, in, out, errOut := iostreams.NewTestIOStreams()
			in.WriteString(tt.input)

			textPrinter, _ := printer.NewPrinter(printer.FormatText)

			o := &DevRemoveOptions{
				BundlePath: existingHCLFile,
				Prompt:     true,
				Config:     &bundle.UDSBundleConfig{Options: &defaults},
				Printer:    textPrinter,
				IOStreams:  streams,
			}

			require.NoError(t, o.Validate())
			err := o.Run(t.Context())
			require.NoError(t, err)
			assert.Empty(t, out.String(), "stdout should be empty when removal is cancelled")
			for _, expected := range tt.wantErrOutput {
				assert.Contains(t, errOut.String(), expected)
			}
		})
	}
}

func TestRemoveOptions_PackagesFlag(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()

	bundleCmd := NewBundleCommand(streams)
	removeCmd, _, _ := bundleCmd.Find([]string{"remove"})
	require.NotNil(t, removeCmd)

	packagesFlag := removeCmd.Flags().Lookup("packages")
	require.NotNil(t, packagesFlag, "packages flag should be defined on remove command")
	assert.Equal(t, "p", packagesFlag.Shorthand)
}

func TestDevRemoveOptions_PackagesFlag(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()

	bundleCmd := NewBundleCommand(streams)
	removeCmd, _, _ := bundleCmd.Find([]string{"dev", "remove"})
	require.NotNil(t, removeCmd)

	packagesFlag := removeCmd.Flags().Lookup("packages")
	require.NotNil(t, packagesFlag, "packages flag should be defined on dev remove command")
	assert.Equal(t, "p", packagesFlag.Shorthand)
}

func TestRemoveOptions_Complete_WithPackagesFlag(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()

	bundleCmd := NewBundleCommand(streams)
	removeCmd, _, _ := bundleCmd.Find([]string{"remove"})
	require.NotNil(t, removeCmd)

	o := NewRemoveOptions(streams)
	// Simulate setting the flag
	require.NoError(t, removeCmd.Flags().Set("packages", "nginx,podinfo"))
	err := o.Complete(removeCmd, []string{"oci://example.com/bundle:v1"})
	require.NoError(t, err)
	// The flag is on the Options struct, not parsed from Complete
	// but we can verify Complete doesn't error
	assert.Equal(t, "oci://example.com/bundle:v1", o.BundlePath)
}

func TestDevRemoveOptions_Complete_WithPackagesFlag(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()

	bundleCmd := NewBundleCommand(streams)
	removeCmd, _, _ := bundleCmd.Find([]string{"dev", "remove"})
	require.NotNil(t, removeCmd)

	o := NewDevRemoveOptions(streams)
	// Simulate setting the flag
	require.NoError(t, removeCmd.Flags().Set("packages", "nginx,podinfo"))
	err := o.Complete(removeCmd, []string{"."})
	require.NoError(t, err)
	// The flag is on the Options struct, not parsed from Complete
	// but we can verify Complete doesn't error
	assert.Equal(t, ".", o.BundlePath)
}

func createRemoveDependencyFixtures(t *testing.T) (string, string) {
	t.Helper()

	root := t.TempDir()
	for _, name := range []string{"base", "leaf"} {
		packageDir := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(packageDir, 0o755))
		zarfConfig := "kind: ZarfPackageConfig\nmetadata:\n  name: " + name + "\n  version: 1.0.0\n  aggregateChecksum: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\ncomponents: []\n"
		require.NoError(t, os.WriteFile(filepath.Join(packageDir, "zarf.yaml"), []byte(zarfConfig), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(packageDir, "checksums.txt"), nil, 0o600))
	}

	bundlePath := filepath.Join(root, bundleFileName)
	require.NoError(t, os.WriteFile(bundlePath, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "remove-dependency-test"
  version = "1.0.0"
}
package "base" {
  source = "base"
  signature_verification { verify = false }
}
package "leaf" {
  source = "leaf"
  signature_verification { verify = false }
  depends_on = [package.base]
}
`), 0o600))

	defaults := NewConfigResolver().Defaults()
	defaults.TmpDir = t.TempDir()
	result, err := bundle.Create(t.Context(), bundlePath, bundle.CreateOptions{
		Config: &bundle.UDSBundleConfig{Options: &defaults},
		Signing: bundle.SigningOptions{
			Mode: bundle.SigningModeUnsigned,
		},
		Streams: iostreams.IOStreams{},
	})
	require.NoError(t, err)

	return bundlePath, result.OutputPath
}

func TestRemoveOptions_Validate_DependencyCheck(t *testing.T) {
	_, artifactPath := createRemoveDependencyFixtures(t)
	defaults := NewConfigResolver().Defaults()

	tests := []struct {
		name     string
		packages []string
		force    bool
		wantErr  string
	}{
		{
			name: "no --packages skips safety check",
		},
		{
			name:     "removing leaf is safe without --force",
			packages: []string{"leaf"},
		},
		{
			name:     "removing dependency without --force is blocked",
			packages: []string{"base"},
			wantErr:  `"base" is required by: leaf`,
		},
		{
			name:     "removing dependency with --force is allowed",
			packages: []string{"base"},
			force:    true,
		},
		{
			name:     "removing the entire chain is safe",
			packages: []string{"base", "leaf"},
		},
		{
			name:     "unknown package surfaces ValidatePackageNames before safety check",
			packages: []string{"bogus"},
			wantErr:  "unknown packages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, in, _, _ := iostreams.NewTestIOStreams()
			in.WriteString("n\n")
			o := &RemoveOptions{
				BundlePath:   artifactPath,
				Packages:     tt.packages,
				Force:        tt.force,
				Prompt:       true,
				Config:       &bundle.UDSBundleConfig{Options: &defaults},
				IOStreams:    streams,
				Verification: VerifyOptions{SkipSignatureVerification: true},
			}

			require.NoError(t, o.Validate())
			err := o.Run(t.Context())
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestDevRemoveOptions_Validate_DependencyCheck(t *testing.T) {
	bundlePath, _ := createRemoveDependencyFixtures(t)
	defaults := NewConfigResolver().Defaults()

	tests := []struct {
		name     string
		packages []string
		force    bool
		wantErr  string
	}{
		{
			name: "no --packages skips safety check",
		},
		{
			name:     "removing leaf is safe without --force",
			packages: []string{"leaf"},
		},
		{
			name:     "removing dependency without --force is blocked",
			packages: []string{"base"},
			wantErr:  `"base" is required by: leaf`,
		},
		{
			name:     "removing dependency with --force is allowed",
			packages: []string{"base"},
			force:    true,
		},
		{
			name:     "removing the entire chain is safe",
			packages: []string{"base", "leaf"},
		},
		{
			name:     "unknown package surfaces ValidatePackageNames before safety check",
			packages: []string{"bogus"},
			wantErr:  "unknown packages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, in, _, _ := iostreams.NewTestIOStreams()
			in.WriteString("n\n")
			o := &DevRemoveOptions{
				BundlePath: bundlePath,
				Packages:   tt.packages,
				Force:      tt.force,
				Prompt:     true,
				Config:     &bundle.UDSBundleConfig{Options: &defaults},
				IOStreams:  streams,
			}

			require.NoError(t, o.Validate())
			err := o.Run(t.Context())
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// TestRemoveOptions_ForceFlag verifies the --force/-f flag is wired on the cobra
// command and bound to RemoveOptions.Force.
func TestRemoveOptions_ForceFlag(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()

	bundleCmd := NewBundleCommand(streams)
	removeCmd, _, _ := bundleCmd.Find([]string{"remove"})
	require.NotNil(t, removeCmd)

	forceFlag := removeCmd.Flags().Lookup("force")
	require.NotNil(t, forceFlag, "force flag should be defined on remove command")
	assert.Equal(t, "false", forceFlag.DefValue, "force should default to false")
	assert.Equal(t, "f", forceFlag.Shorthand, "force should have -f as shorthand")
}

// TestDevRemoveOptions_ForceFlag verifies the --force/-f flag is wired on the
// dev cobra command and bound to DevRemoveOptions.Force.
func TestDevRemoveOptions_ForceFlag(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()

	bundleCmd := NewBundleCommand(streams)
	removeCmd, _, _ := bundleCmd.Find([]string{"dev", "remove"})
	require.NotNil(t, removeCmd)

	forceFlag := removeCmd.Flags().Lookup("force")
	require.NotNil(t, forceFlag, "force flag should be defined on dev remove command")
	assert.Equal(t, "false", forceFlag.DefValue, "force should default to false")
	assert.Equal(t, "f", forceFlag.Shorthand, "force should have -f as shorthand")
}
