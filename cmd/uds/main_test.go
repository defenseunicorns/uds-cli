// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/mode"
	"github.com/stretchr/testify/require"
)

func TestRunRejectsUnavailableNextWithoutLoadingLegacyConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "uds-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("options:\n  invalid: true\n"), 0o600))
	t.Setenv("UDS_CONFIG", configPath)

	err := run([]string{"--cli-mode", "next", "version"})

	require.EqualError(t, err, `CLI mode "next" is not available in this build`)
	require.Equal(t, mode.Next.String(), os.Getenv(mode.EnvName))
}
