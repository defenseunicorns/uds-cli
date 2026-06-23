// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/printer"
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

			bundleCmd := NewBundleCommand(streams)
			cmd, _, _ := bundleCmd.Find([]string{"deploy"})
			err := o.Complete(cmd, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBundlePath, o.BundlePath)
		})
	}
}

func TestDeployOptions_Validate(t *testing.T) {
	// Use the existing deploy/init bundle for valid file test case
	existingFile := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", bundle.BundleFileName)
	existingDir := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init")

	// Create temporary test files and directories
	tempDir := t.TempDir()

	// Create an empty directory (no bundle.uds.hcl)
	emptyDir := filepath.Join(tempDir, "empty")
	require.NoError(t, os.Mkdir(emptyDir, 0o755))

	// Create a valid directory with bundle.uds.hcl
	validDir := filepath.Join(tempDir, "valid")
	require.NoError(t, os.Mkdir(validDir, 0o755))
	validBundleFile := filepath.Join(validDir, bundle.BundleFileName)
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
			name:       "tar.zst archive that exists",
			bundlePath: tarZstFile,
			wantErr:    "",
		},
		{
			name:       "tar.zst archive path that doesn't exist",
			bundlePath: "bundle.tar.zst",
			wantErr:    "bundle artifact not found",
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
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestDeployOptions_Run_PromptDecline(t *testing.T) {
	// Test the --prompt flag behavior: user declines deployment.
	// Non-interactive (default) Run tests that proceed to actual deployment
	// are covered by integration tests since they require OCI registries.
	existingFile := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", bundle.BundleFileName)

	tests := []struct {
		name          string
		bundlePath    string
		input         string   // Simulated user input for confirmation prompt
		wantErrOutput []string // Strings that should appear in stderr output
	}{
		{
			name:       "prompt flag - user confirms no",
			bundlePath: existingFile,
			input:      "n\n",
			wantErrOutput: []string{
				"Deploy this bundle?",
			},
		},
		{
			name:       "prompt flag - empty input treated as no",
			bundlePath: existingFile,
			input:      "\n",
			wantErrOutput: []string{
				"Deploy this bundle?",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, in, out, errOut := iostreams.NewTestIOStreams()
			in.WriteString(tt.input)
			textPrinter, _ := printer.NewPrinter(printer.FormatText)

			// Wire a cobra cmd with --prompt set so Run's Resolve picks it up.
			bundleCmd := NewBundleCommand(streams)
			bundleCmd.PersistentFlags().Bool("prompt", false, "enable interactive confirmation prompts")
			deployCmd, _, err := bundleCmd.Find([]string{"deploy"})
			require.NoError(t, err)
			require.NoError(t, deployCmd.ParseFlags([]string{"--prompt"}))

			o := &DeployOptions{
				BundlePath: tt.bundlePath,
				Printer:    textPrinter,
				flags:      SnapshotFlags(deployCmd),
				IOStreams:  streams,
			}

			err = o.Run(t.Context())
			require.NoError(t, err)
			// Stdout should be empty when deployment is cancelled (no result printed)
			assert.Empty(t, out.String(), "stdout should be empty when deployment is cancelled")
			for _, expected := range tt.wantErrOutput {
				assert.Contains(t, errOut.String(), expected)
			}
		})
	}
}

func TestDeployOptions_NoninteractivePrompt(t *testing.T) {
	// Prompt defaults to false via GlobalOptions (non-interactive).
	// With the new design, prompt is read from Config.Global.Prompt
	// which defaults to false when the --prompt flag is not set.
	global := &bundle.GlobalOptions{}
	assert.False(t, global.Prompt, "Prompt should default to false (non-interactive)")
}

func TestPromptConfirmation(t *testing.T) {
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

			confirmed, err := PromptConfirmation(streams, "Test this?")

			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantYes, confirmed)
			}
		})
	}
}

func TestPromptConfirmation_BrokenReader(t *testing.T) {
	brokenErr := fmt.Errorf("read error: disk failure")
	streams := iostreams.New(
		&brokenReader{err: brokenErr},
		&bytes.Buffer{},
		&bytes.Buffer{},
	)

	confirmed, err := PromptConfirmation(streams, "Test this?")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading confirmation")
	assert.False(t, confirmed)
}

// brokenReader always returns an error from Read.
type brokenReader struct {
	err error
}

func (r *brokenReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

// ensure brokenReader implements io.Reader.
var _ io.Reader = (*brokenReader)(nil)

func TestDeployOptions_Flags(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()

	// Verify --config flag is inherited from parent bundle command
	bundleCmd := NewBundleCommand(streams)
	configFlag := bundleCmd.PersistentFlags().Lookup("config")
	require.NotNil(t, configFlag, "config flag should be defined on parent bundle command")
	assert.Empty(t, configFlag.DefValue)
}

// TestDeployOptions_PackagesForceFlags verifies that --packages/-p and --force/-f
// are wired on the deploy command and bound to DeployOptions.
func TestDeployOptions_PackagesForceFlags(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()

	bundleCmd := NewBundleCommand(streams)
	deployCmd, _, _ := bundleCmd.Find([]string{"deploy"})
	require.NotNil(t, deployCmd)

	packagesFlag := deployCmd.Flags().Lookup("packages")
	require.NotNil(t, packagesFlag, "packages flag should be defined on deploy command")
	assert.Equal(t, "p", packagesFlag.Shorthand)

	forceFlag := deployCmd.Flags().Lookup("force")
	require.NotNil(t, forceFlag, "force flag should be defined on deploy command")
	assert.Equal(t, "f", forceFlag.Shorthand)
	assert.Equal(t, "false", forceFlag.DefValue, "force should default to false")
}
