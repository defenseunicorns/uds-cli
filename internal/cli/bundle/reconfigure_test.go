// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconfigureCommand_MissingArgs(t *testing.T) {
	t.Parallel()
	streams, _, _, _ := iostreams.NewTestIOStreams()
	cmd := NewBundleCommand(streams)
	cmd.SetArgs([]string{"reconfigure"})
	err := cmd.Execute()
	require.Error(t, err)
}

func TestReconfigureCommand_MissingDefaultsFlag(t *testing.T) {
	t.Parallel()
	streams, _, _, _ := iostreams.NewTestIOStreams()
	cmd := NewBundleCommand(streams)
	cmd.SetArgs([]string{"reconfigure", "/some/bundle.tar.zst"})
	err := cmd.Execute()
	require.ErrorContains(t, err, "defaults")
}

func TestReconfigureCommand_DefaultSuffix(t *testing.T) {
	t.Parallel()
	o := NewReconfigureOptions(iostreams.IOStreams{})
	assert.Equal(t, "-reconfigured", o.Suffix)
}

func TestReconfigureCommand_ValidateRejectsEmptySuffix(t *testing.T) {
	t.Parallel()
	defaults := filepath.Join(t.TempDir(), "defaults.uds.hcl")
	require.NoError(t, os.WriteFile(defaults, []byte(`variables = { a = "b" }`), 0o600))

	o := &ReconfigureOptions{
		Source:       "/some/bundle.tar.zst",
		DefaultsFile: defaults,
		Suffix:       "",
	}
	err := o.Validate()
	require.ErrorContains(t, err, "suffix")
}

func TestReconfigureCommand_ValidateRejectsOutputDirForOCI(t *testing.T) {
	t.Parallel()
	defaults := filepath.Join(t.TempDir(), "defaults.uds.hcl")
	require.NoError(t, os.WriteFile(defaults, []byte(`variables = { a = "b" }`), 0o600))

	o := &ReconfigureOptions{
		Source:       "oci://example.com/test/bundle:v1.0.0",
		DefaultsFile: defaults,
		Suffix:       "-test",
		OutputDir:    "/some/dir",
	}
	err := o.Validate()
	require.ErrorContains(t, err, "output-dir")
}

func TestReconfigureCommand_ValidateRejectsMissingDefaultsFile(t *testing.T) {
	t.Parallel()
	o := &ReconfigureOptions{
		Source:       "/some/bundle.tar.zst",
		DefaultsFile: "/nonexistent/defaults.uds.hcl",
		Suffix:       "-test",
	}
	err := o.Validate()
	require.ErrorContains(t, err, "not found")
}

func TestReconfigureCommand_ValidateRequiresVerificationPolicy(t *testing.T) {
	t.Parallel()
	defaults := filepath.Join(t.TempDir(), "defaults.uds.hcl")
	require.NoError(t, os.WriteFile(defaults, []byte(`variables = { a = "b" }`), 0o600))

	o := &ReconfigureOptions{
		Source:       "/some/bundle.tar.zst",
		DefaultsFile: defaults,
		Suffix:       "-test",
		Signing:      bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: "cosign.key"},
	}
	err := o.Validate()
	require.ErrorContains(t, err, "signature verification must configure exactly one of public key or keyless")
}

func TestReconfigureOptions_Run_PromptDecline(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	defaults := filepath.Join(tempDir, "defaults.uds.hcl")
	require.NoError(t, os.WriteFile(defaults, []byte(`variables = { a = "b" }`), 0o600))

	tests := []struct {
		name          string
		input         string
		wantErrOutput []string
	}{
		{
			name:  "prompt flag - user confirms no",
			input: "n\n",
			wantErrOutput: []string{
				"Reconfigure this bundle?",
			},
		},
		{
			name:  "prompt flag - empty input treated as no",
			input: "\n",
			wantErrOutput: []string{
				"Reconfigure this bundle?",
			},
		},
	}

	defaults_cfg := NewConfigResolver().Defaults()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, in, out, errOut := iostreams.NewTestIOStreams()
			in.WriteString(tt.input)

			textPrinter, _ := printer.NewPrinter(printer.FormatText)

			o := &ReconfigureOptions{
				Source:       "/some/bundle.tar.zst",
				DefaultsFile: defaults,
				Suffix:       "-test",
				Prompt:       true,
				Config:       &bundle.UDSBundleConfig{Options: &defaults_cfg},
				Printer:      textPrinter,
				IOStreams:    streams,
			}

			err := o.Run(t.Context())
			require.NoError(t, err)
			assert.Empty(t, out.String(), "stdout should be empty when reconfigure is cancelled")
			for _, expected := range tt.wantErrOutput {
				assert.Contains(t, errOut.String(), expected)
			}
		})
	}
}
