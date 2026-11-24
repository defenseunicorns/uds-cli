// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package tools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClearCacheMasksMarkedCachePathInPrompt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.uds.hcl")
	require.NoError(t, os.WriteFile(path, []byte(`options { uds_cache = sensitive(file("cache-dir")) }`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cache-dir"), []byte(filepath.Join(dir, "private-cache")), 0o600))
	streams, in, _, diagnostics := iostreams.NewTestIOStreams()
	in.WriteString("n\n")
	cmd := newClearCacheCommand(streams)
	cmd.Flags().Bool("prompt", false, "")
	cmd.SetArgs([]string{"--config", path, "--prompt", "--uds-cache", filepath.Join(dir, "private-override")})
	require.NoError(t, cmd.ExecuteContext(t.Context()))
	assert.Contains(t, diagnostics.String(), "***")
	assert.NotContains(t, diagnostics.String(), "private-override")
}
