// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/printer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDevDeployOptions_Complete(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	bundleCmd := NewBundleCommand(streams)
	cmd, _, err := bundleCmd.Find([]string{"dev", "deploy"})
	require.NoError(t, err)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "defaults to current directory", want: "."},
		{name: "source directory", args: []string{"./bundle"}, want: "./bundle"},
		{name: "source file", args: []string{"./bundle/bundle.uds.hcl"}, want: "./bundle/bundle.uds.hcl"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := NewDevDeployOptions(streams)
			require.NoError(t, o.Complete(cmd, tt.args))
			assert.Equal(t, tt.want, o.BundlePath)
			assert.NotNil(t, o.Printer)
		})
	}
}

func TestDevDeployOptions_Validate(t *testing.T) {
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	require.NoError(t, os.Mkdir(sourceDir, 0o700))
	sourceFile := filepath.Join(sourceDir, bundlepkg.BundleFileName)
	require.NoError(t, os.WriteFile(sourceFile, []byte("test"), 0o600))
	artifact := filepath.Join(tempDir, "bundle.tar.zst")
	require.NoError(t, os.WriteFile(artifact, []byte("test"), 0o600))

	tests := []struct {
		name    string
		ref     string
		wantErr string
	}{
		{name: "source directory", ref: sourceDir},
		{name: "source file", ref: sourceFile},
		{name: "local artifact", ref: artifact, wantErr: "uds bundle deploy"},
		{name: "OCI artifact", ref: "oci://ghcr.io/example/bundle:1.0.0", wantErr: "uds bundle deploy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &DevDeployOptions{BundlePath: tt.ref}
			err := o.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestDevDeployOptions_Run_DiagnosticIsUnfilteredAndPromptDeclines(t *testing.T) {
	sourceFile := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", bundlepkg.BundleFileName)

	tests := []struct {
		name       string
		flags      CLIFlags
		configBody string
	}{
		{
			name:  "CLI error log level",
			flags: CLIFlags{LogLevel: "error", LogLevelChanged: true, Prompt: true},
		},
		{
			name:       "HCL error log level",
			flags:      CLIFlags{Prompt: true},
			configBody: "options { log_level = \"error\" }\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, in, out, errOut := iostreams.NewTestIOStreams()
			in.WriteString("n\n")
			flags := tt.flags
			tmpDir := t.TempDir()
			flags.TmpDir = tmpDir
			flags.TmpDirChanged = true
			if tt.configBody != "" {
				configPath := filepath.Join(t.TempDir(), "config.uds.hcl")
				require.NoError(t, os.WriteFile(configPath, []byte(tt.configBody), 0o600))
				flags.ConfigPath = configPath
			}
			textPrinter, err := printer.NewPrinter(printer.FormatText)
			require.NoError(t, err)

			o := &DevDeployOptions{
				BundlePath: sourceFile,
				Printer:    textPrinter,
				flags:      flags,
				IOStreams:  streams,
			}
			require.NoError(t, o.Run(t.Context()))
			assert.Contains(t, errOut.String(), "bundle definition")
			assert.Contains(t, errOut.String(), "bundle provenance")
			assert.Contains(t, errOut.String(), "bundle-signature verification")
			assert.Contains(t, errOut.String(), "Deploy this bundle?")
			assert.Empty(t, out.String())
			entries, err := os.ReadDir(tmpDir)
			require.NoError(t, err)
			assert.Empty(t, entries, "development deploy must not create an intermediate artifact")
		})
	}
}

func TestDevDeployOptions_Run_ResolvesTarZstDirectoryAsSource(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	sourceDir := filepath.Join(t.TempDir(), "source.tar.zst")
	require.NoError(t, os.Mkdir(sourceDir, 0o700))
	bundlePath := filepath.Join(sourceDir, bundlepkg.BundleFileName)
	require.NoError(t, os.WriteFile(bundlePath, []byte("test"), 0o600))
	textPrinter, err := printer.NewPrinter(printer.FormatText)
	require.NoError(t, err)

	var gotPath string
	o := &DevDeployOptions{
		BundlePath: sourceDir,
		Printer:    textPrinter,
		IOStreams:  streams,
		runDeploy: func(_ context.Context, _ iostreams.IOStreams, _ *bundlepkg.UDSBundleConfig, path string, _ []string, _ bool) (*bundlepkg.DeployResult, error) {
			gotPath = path
			return nil, nil
		},
	}

	require.NoError(t, o.Run(t.Context()))
	assert.Equal(t, bundlePath, gotPath)
}

func TestNewDevCommand_ContainsDeploy(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	cmd := NewDevCommand(streams)
	found, _, err := cmd.Find([]string{"deploy"})
	require.NoError(t, err)
	assert.Equal(t, "deploy", found.Name())
}
