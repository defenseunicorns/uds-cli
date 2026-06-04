// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "defaults")
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "suffix")
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output-dir")
}

func TestReconfigureCommand_ValidateRejectsMissingDefaultsFile(t *testing.T) {
	t.Parallel()
	o := &ReconfigureOptions{
		Source:       "/some/bundle.tar.zst",
		DefaultsFile: "/nonexistent/defaults.uds.hcl",
		Suffix:       "-test",
	}
	err := o.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
