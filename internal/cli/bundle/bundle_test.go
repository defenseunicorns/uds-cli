// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestCommandOptions_CompletePropagatesPrompt(t *testing.T) {
	defaultsFile := filepath.Join(t.TempDir(), bundleDefaultsFileName)
	require.NoError(t, os.WriteFile(defaultsFile, []byte(`variables = {}`), 0o600))
	streams := iostreams.IOStreams{}

	tests := []struct {
		name     string
		setup    func(*cobra.Command)
		complete func(*cobra.Command) (bool, error)
	}{
		{
			name: "create",
			complete: func(cmd *cobra.Command) (bool, error) {
				o := NewCreateOptions(streams)
				err := o.Complete(cmd, []string{"."})
				return o.Prompt, err
			},
		},
		{
			name: "pull",
			complete: func(cmd *cobra.Command) (bool, error) {
				o := NewPullOptions(streams)
				err := o.Complete(cmd, []string{"oci://example.com/bundle:v1"})
				return o.Prompt, err
			},
		},
		{
			name: "push",
			complete: func(cmd *cobra.Command) (bool, error) {
				o := NewPushOptions(streams)
				err := o.Complete(cmd, []string{"bundle.tar.zst", "oci://example.com/bundle:v1"})
				return o.Prompt, err
			},
		},
		{
			name: "reconfigure",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().String("defaults", defaultsFile, "")
				cmd.Flags().String("suffix", "-reconfigured", "")
			},
			complete: func(cmd *cobra.Command) (bool, error) {
				o := NewReconfigureOptions(streams)
				err := o.Complete(cmd, []string{"bundle.tar.zst"})
				return o.Prompt, err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			if tt.setup != nil {
				tt.setup(cmd)
			}
			cmd.Flags().Bool("prompt", false, "")
			require.NoError(t, cmd.Flags().Set("prompt", "true"))

			prompt, err := tt.complete(cmd)
			require.NoError(t, err)
			assert.True(t, prompt)
		})
	}
}

func TestPullOptions_Validate(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	tests := []struct {
		name         string
		ociReference string
		outputDir    string
		wantErr      string
	}{
		{
			name:         "empty OCI reference",
			ociReference: "",
			wantErr:      "OCI reference is required",
		},
		{
			name:         "valid inputs",
			ociReference: "ghcr.io/test:v1",
			outputDir:    tmpDir,
		},
		{
			name:         "empty output dir is allowed (defaults to cwd)",
			ociReference: "ghcr.io/test:v1",
			outputDir:    "",
		},
		{
			name:         "non-existent output dir",
			ociReference: "ghcr.io/test:v1",
			outputDir:    tmpDir + "/nonexistent",
			wantErr:      "--output-dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			defaults := NewConfigResolver().Defaults()
			o := &PullOptions{
				OCIReference: tt.ociReference,
				OutputDir:    tt.outputDir,
				Config:       &bundle.UDSBundleConfig{Options: &defaults},
				Verification: VerifyOptions{SkipSignatureVerification: true},
			}
			err := o.Validate()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
func TestPushOptions_Validate(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	validTarball := filepath.Join(tmpDir, "bundle.tar.zst")
	require.NoError(t, os.WriteFile(validTarball, []byte("fake"), 0o600))

	tests := []struct {
		name         string
		tarball      string
		ociReference string
		wantErr      string
	}{
		{
			name:         "valid inputs",
			tarball:      validTarball,
			ociReference: "ghcr.io/org/bundle:v1",
		},
		{
			name:         "tarball with wrong extension",
			tarball:      "bundle.zip",
			ociReference: "ghcr.io/org/bundle:v1",
			wantErr:      "source must be a .tar.zst bundle file",
		},
		{
			name:         "tarball does not exist",
			tarball:      filepath.Join(tmpDir, "missing.tar.zst"),
			ociReference: "ghcr.io/org/bundle:v1",
			wantErr:      "bundle file not found",
		},
		{
			name:         "empty OCI reference",
			tarball:      validTarball,
			ociReference: "",
			wantErr:      "OCI reference is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			defaults := NewConfigResolver().Defaults()
			o := &PushOptions{
				Tarball:      tt.tarball,
				OCIReference: tt.ociReference,
				Config:       &bundle.UDSBundleConfig{Options: &defaults},
			}
			err := o.Validate()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPullOptions_Complete(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := &PullOptions{IOStreams: streams}
	cmd := &cobra.Command{}

	err := o.Complete(cmd, []string{"ghcr.io/test:v1"})
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/test:v1", o.OCIReference)
}

func TestPushOptions_Complete(t *testing.T) {
	t.Parallel()
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := &PushOptions{IOStreams: streams}
	cmd := &cobra.Command{}

	err := o.Complete(cmd, []string{"bundle.tar.zst", "ghcr.io/org/bundle:v1"})
	require.NoError(t, err)
	assert.Equal(t, "bundle.tar.zst", o.Tarball)
	assert.Equal(t, "ghcr.io/org/bundle:v1", o.OCIReference)
}
