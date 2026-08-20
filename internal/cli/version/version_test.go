// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/internal/version"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestVersionOptions_Run(t *testing.T) {
	// Save original values
	origVersion := version.Version
	origGitCommit := version.GitCommit
	origBuildDate := version.BuildDate

	// Set test values
	version.Version = "test-version"
	version.GitCommit = "abc123"
	version.BuildDate = "2026-02-03"

	// Restore original values after test
	defer func() {
		version.Version = origVersion
		version.GitCommit = origGitCommit
		version.BuildDate = origBuildDate
	}()

	streams, _, out, _ := iostreams.NewTestIOStreams()

	o := &Options{
		IOStreams: streams,
	}

	err := o.Run()
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "test-version")
	assert.Contains(t, output, "abc123")
	assert.Contains(t, output, "2026-02-03")
}

func TestVersionOptions_Complete(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := NewOptions(streams)

	cmd := NewVersionCommand(streams)
	err := o.Complete(cmd, []string{})
	require.NoError(t, err)
}

func TestVersionOptions_Validate(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := NewOptions(streams)

	err := o.Validate()
	require.NoError(t, err)
}
