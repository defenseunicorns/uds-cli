// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cli

package bundle_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

func TestExecuteCLI_CacheIsolation(t *testing.T) {
	ref := testutil.StartLocalRegistry(t) + "/test/cache-isolation:v1"
	pushBundleArtifact(t, createInspectArtifact(t), ref, &bundlepkg.UDSBundleConfig{
		Options: &bundlepkg.ConfigOptions{PlainHTTP: true, Concurrency: 1},
	})

	// A file cannot be a cache directory. This safely catches fallback to HCL
	// without allowing a regression to write to the real user's default cache.
	blockedCache := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blockedCache, []byte("preserve"), 0o600))
	config := filepath.Join(t.TempDir(), "config.uds.hcl")
	require.NoError(t, os.WriteFile(config, fmt.Appendf(nil, "options { uds_cache = %q }\n", blockedCache), 0o600))

	for _, name := range []string{"implicit temporary cache", "separate flag", "equals flag", "empty separate flag retains HCL", "empty equals flag retains HCL"} {
		t.Run(name, func(t *testing.T) {
			args := []string{"bundle", "inspect", ref, "--plain-http", "--config", config}
			cache := t.TempDir()
			switch name {
			case "separate flag":
				args = append(args, "--uds-cache", cache)
			case "equals flag":
				args = append(args, "--uds-cache="+cache)
			case "empty separate flag retains HCL":
				args = append(args, "--uds-cache", "")
			case "empty equals flag retains HCL":
				args = append(args, "--uds-cache=")
			}
			_, err := executeCLI(t, "", args...)
			if name == "empty separate flag retains HCL" || name == "empty equals flag retains HCL" {
				require.Error(t, err)
				require.Contains(t, err.Error(), blockedCache)
				require.Contains(t, err.Error(), "not a directory")
				return
			}
			require.NoError(t, err)
			if name == "separate flag" || name == "equals flag" {
				require.DirExists(t, filepath.Join(cache, "bundle-oci", "images"))
			}
		})
	}
	contents, err := os.ReadFile(blockedCache)
	require.NoError(t, err)
	require.Equal(t, "preserve", string(contents))
}
