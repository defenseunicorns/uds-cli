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

func TestDeployOptions_Complete(t *testing.T) {
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
			o := NewDeployOptions(streams)

			cmd := NewDeployCommand(streams)
			err := o.Complete(cmd, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBundlePath, o.BundlePath)
		})
	}
}

func TestDeployOptions_Validate(t *testing.T) {
	// Use the existing deploy/init bundle for valid file test case
	existingFile := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", BundleFileName)
	existingDir := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init")

	// Create temporary test files and directories
	tempDir := t.TempDir()

	// Create an empty directory (no bundle.uds.hcl)
	emptyDir := filepath.Join(tempDir, "empty")
	require.NoError(t, os.Mkdir(emptyDir, 0o755))

	// Create a valid directory with bundle.uds.hcl
	validDir := filepath.Join(tempDir, "valid")
	require.NoError(t, os.Mkdir(validDir, 0o755))
	validBundleFile := filepath.Join(validDir, BundleFileName)
	require.NoError(t, os.WriteFile(validBundleFile, []byte(`
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "test" }
package "pkg1" { source = "oci://example.com/pkg:v1" }
`), 0o644))

	// Create a tar.zst file
	tarZstFile := filepath.Join(tempDir, "bundle.tar.zst")
	require.NoError(t, os.WriteFile(tarZstFile, []byte("test"), 0o644))

	// Create an HCL file with wrong name
	wrongNameFile := filepath.Join(tempDir, "wrongname.hcl")
	require.NoError(t, os.WriteFile(wrongNameFile, []byte("test"), 0o644))

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
			wantErr:    "tar.zst bundles not yet supported",
		},
		{
			name:       "tar.zst archive path that doesn't exist",
			bundlePath: "bundle.tar.zst",
			wantErr:    "tar.zst bundles not yet supported",
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
			name:       "directory without bundle.uds.hcl",
			bundlePath: emptyDir,
			wantErr:    "directory does not contain bundle.uds.hcl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			o := &DeployOptions{
				BundlePath: tt.bundlePath,
				IOStreams:  streams,
			}

			err := o.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				// Validate should not modify BundlePath
				assert.Equal(t, tt.bundlePath, o.BundlePath, "Validate() should not modify BundlePath")
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestDeployOptions_Run_PromptDecline(t *testing.T) {
	// Test the --prompt flag behavior: user declines deployment.
	// Non-interactive (default) Run tests that proceed to actual deployment
	// are covered by integration tests since they require OCI registries.
	existingFile := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", BundleFileName)

	tests := []struct {
		name       string
		bundlePath string
		input      string   // Simulated user input for confirmation prompt
		wantOutput []string // Strings that should appear in output
	}{
		{
			name:       "prompt flag - user confirms no",
			bundlePath: existingFile,
			input:      "n\n",
			wantOutput: []string{
				"k3d-core-init",
				"uds_k3d_dev",
				"init",
				"Deploy this bundle?",
				"Deployment cancelled",
			},
		},
		{
			name:       "prompt flag - empty input treated as no",
			bundlePath: existingFile,
			input:      "",
			wantOutput: []string{
				"Deployment cancelled",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, in, out, _ := iostreams.NewTestIOStreams()
			in.WriteString(tt.input)

			o := &DeployOptions{
				BundlePath: tt.bundlePath,
				Prompt:     true,
				IOStreams:  streams,
			}

			err := o.Run()
			require.NoError(t, err)
			for _, expected := range tt.wantOutput {
				assert.Contains(t, out.String(), expected)
			}
		})
	}
}

func TestDeployOptions_NoninteractivePrompt(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := NewDeployOptions(streams)

	assert.False(t, o.Prompt, "Prompt should default to false (non-interactive)")
}

func TestDeployOptions_PromptConfirmation(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantYes   bool
		wantError bool
	}{
		{name: "y confirms", input: "y\n", wantYes: true},
		{name: "Y confirms", input: "Y\n", wantYes: true},
		{name: "yes confirms", input: "yes\n", wantYes: true},
		{name: "YES confirms", input: "YES\n", wantYes: true},
		{name: "n declines", input: "n\n", wantYes: false},
		{name: "N declines", input: "N\n", wantYes: false},
		{name: "no declines", input: "no\n", wantYes: false},
		{name: "empty declines", input: "\n", wantYes: false},
		{name: "random text declines", input: "maybe\n", wantYes: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, in, _, _ := iostreams.NewTestIOStreams()
			in.WriteString(tt.input)

			o := &DeployOptions{IOStreams: streams}
			confirmed, err := o.promptConfirmation()

			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantYes, confirmed)
			}
		})
	}
}

func TestDeployOptions_Flags(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	cmd := NewDeployCommand(streams)

	// Verify --prompt flag is defined
	promptFlag := cmd.Flags().Lookup("prompt")
	require.NotNil(t, promptFlag, "prompt flag should be defined")
	assert.Equal(t, "false", promptFlag.DefValue, "prompt should default to false (non-interactive)")
}
