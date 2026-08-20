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
			o := NewRemoveOptions(streams)

			bundleCmd := NewBundleCommand(streams)
			cmd, _, _ := bundleCmd.Find([]string{"remove"})
			err := o.Complete(cmd, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBundlePath, o.BundlePath)
		})
	}
}

func TestRemoveOptions_Validate(t *testing.T) {
	existingFile := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", bundleFileName)
	existingDir := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init")

	tempDir := t.TempDir()

	emptyDir := filepath.Join(tempDir, "empty")
	require.NoError(t, os.Mkdir(emptyDir, 0o755))

	validDir := filepath.Join(tempDir, "valid")
	require.NoError(t, os.Mkdir(validDir, 0o755))
	validBundleFile := filepath.Join(validDir, bundleFileName)
	require.NoError(t, os.WriteFile(validBundleFile, []byte(`
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "test" }
package "pkg1" {
  source = "oci://example.com/pkg:v1"
  signature_verification { verify = false }
}
`), 0o600))

	tarZstFile := filepath.Join(tempDir, "bundle.tar.zst")
	require.NoError(t, os.WriteFile(tarZstFile, []byte("test"), 0o600))

	defaults := NewConfigResolver().Defaults()

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
			name:       "tar.zst archive",
			bundlePath: tarZstFile,
			wantErr:    "tar.zst bundles are not supported",
		},
		{
			name:       "HCL file that does not exist",
			bundlePath: "/nonexistent/bundle.uds.hcl",
			wantErr:    "bundle path not found",
		},
		{
			name:       "directory without bundle.uds.hcl",
			bundlePath: emptyDir,
			wantErr:    "directory does not contain bundle.uds.hcl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			o := &RemoveOptions{
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
	existingFile := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", bundleFileName)
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
				BundlePath: existingFile,
				Prompt:     true,
				Config:     &bundle.UDSBundleConfig{Options: &defaults},
				Printer:    textPrinter,
				IOStreams:  streams,
			}

			// Validate() populates o.parsedBundle, which Run() consumes.
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

func TestRemoveOptions_Complete_WithPackagesFlag(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()

	bundleCmd := NewBundleCommand(streams)
	removeCmd, _, _ := bundleCmd.Find([]string{"remove"})
	require.NotNil(t, removeCmd)

	o := NewRemoveOptions(streams)
	// Simulate setting the flag
	require.NoError(t, removeCmd.Flags().Set("packages", "nginx,podinfo"))
	err := o.Complete(removeCmd, []string{"."})
	require.NoError(t, err)
	// The flag is on the Options struct, not parsed from Complete
	// but we can verify Complete doesn't error
	assert.Equal(t, ".", o.BundlePath)
}

func TestRemoveOptions_Validate_DependencyCheck(t *testing.T) {
	bundlePath := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", bundleFileName)
	defaults := NewConfigResolver().Defaults()

	tests := []struct {
		name     string
		packages []string
		force    bool
		wantErr  string
	}{
		{
			name:     "no --packages skips safety check",
			packages: nil,
		},
		{
			name:     "removing leaf is safe without --force",
			packages: []string{"init"},
		},
		{
			name:     "removing dependency without --force is blocked",
			packages: []string{"uds_k3d_dev"},
			wantErr:  `"uds_k3d_dev" is required by: init`,
		},
		{
			name:     "removing dependency with --force is allowed",
			packages: []string{"uds_k3d_dev"},
			force:    true,
		},
		{
			name:     "removing the entire chain is safe",
			packages: []string{"uds_k3d_dev", "init"},
		},
		{
			name:     "unknown package surfaces ValidatePackageNames before safety check",
			packages: []string{"bogus"},
			wantErr:  "unknown packages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			o := &RemoveOptions{
				BundlePath: bundlePath,
				Packages:   tt.packages,
				Force:      tt.force,
				Config:     &bundle.UDSBundleConfig{Options: &defaults},
				IOStreams:  streams,
			}

			err := o.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.NotNil(t, o.parsedBundle, "Validate() should cache the parsed bundle for Run()")
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
