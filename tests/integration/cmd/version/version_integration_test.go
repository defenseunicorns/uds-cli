// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package version_test

import (
	"testing"

	cmdversion "github.com/defenseunicorns/uds-cli/internal/cli/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestVersionCommand_Integration(t *testing.T) {
	streams, _, out, _ := iostreams.NewTestIOStreams()

	cmd := cmdversion.NewVersionCommand(streams)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.NoError(t, err)

	assert.Contains(t, out.String(), "uds version")
}
