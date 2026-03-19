// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

func TestCreateOptions_Validate(t *testing.T) {
	tempDir := t.TempDir()

	// Create a valid directory with bundle.uds.hcl
	validDir := filepath.Join(tempDir, "valid")
	require.NoError(t, os.Mkdir(validDir, 0o755))
	validBundleFile := filepath.Join(validDir, BundleFileName)
	require.NoError(t, os.WriteFile(validBundleFile, []byte("test content"), 0o644))

	// Create an empty directory (no bundle.uds.hcl)
	emptyDir := filepath.Join(tempDir, "empty")
	require.NoError(t, os.Mkdir(emptyDir, 0o755))

	// Create an HCL file with wrong name
	wrongNameFile := filepath.Join(tempDir, "wrongname.hcl")
	require.NoError(t, os.WriteFile(wrongNameFile, []byte("test"), 0o644))

	defaults := NewConfigResolver().Defaults()

	t.Run("empty string", func(t *testing.T) {
		o := &CreateOptions{BundlePath: "", Config: &bundle.UDSBundleConfig{Global: &bundle.GlobalOptions{}, Options: &defaults}}
		require.Error(t, o.Validate())
	})

	t.Run("path not found", func(t *testing.T) {
		o := &CreateOptions{BundlePath: filepath.Join(tempDir, "does", "not", "exist"), Config: &bundle.UDSBundleConfig{Global: &bundle.GlobalOptions{}, Options: &defaults}}
		require.Error(t, o.Validate())
	})

	t.Run("file with wrong name", func(t *testing.T) {
		o := &CreateOptions{BundlePath: wrongNameFile, Config: &bundle.UDSBundleConfig{Global: &bundle.GlobalOptions{}, Options: &defaults}}
		err := o.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected file named 'bundle.uds.hcl'")
	})

	t.Run("directory missing bundle file", func(t *testing.T) {
		o := &CreateOptions{BundlePath: emptyDir, Config: &bundle.UDSBundleConfig{Global: &bundle.GlobalOptions{}, Options: &defaults}}
		err := o.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "directory does not contain bundle.uds.hcl")
	})

	t.Run("valid directory containing bundle file", func(t *testing.T) {
		o := &CreateOptions{BundlePath: validDir, Config: &bundle.UDSBundleConfig{Global: &bundle.GlobalOptions{}, Options: &defaults}}
		require.NoError(t, o.Validate())
		// Validate should not modify BundlePath
		assert.Equal(t, validDir, o.BundlePath, "Validate() should not modify BundlePath")
	})

	t.Run("valid bundle file directly", func(t *testing.T) {
		o := &CreateOptions{BundlePath: validBundleFile, Config: &bundle.UDSBundleConfig{Global: &bundle.GlobalOptions{}, Options: &defaults}}
		require.NoError(t, o.Validate())
	})

	t.Run("OCI reference", func(t *testing.T) {
		o := &CreateOptions{BundlePath: "oci://ghcr.io/test/bundle:v1", Config: &bundle.UDSBundleConfig{Global: &bundle.GlobalOptions{}, Options: &defaults}}
		err := o.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "OCI bundle references not yet supported")
	})

	t.Run("tar.zst archive", func(t *testing.T) {
		o := &CreateOptions{BundlePath: "bundle.tar.zst", Config: &bundle.UDSBundleConfig{Global: &bundle.GlobalOptions{}, Options: &defaults}}
		err := o.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tar.zst bundles not yet supported")
	})
}

func TestCreateOptions_Complete(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := &CreateOptions{IOStreams: streams}
	cmd := &cobra.Command{}

	err := o.Complete(cmd, []string{"my-bundle-dir"})
	require.NoError(t, err)
	assert.Equal(t, "my-bundle-dir", o.BundlePath)
}

func TestCreateOptions_Complete_NoArgs(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := &CreateOptions{IOStreams: streams}
	cmd := &cobra.Command{}

	err := o.Complete(cmd, []string{})
	require.NoError(t, err)
	assert.Equal(t, ".", o.BundlePath)
}
