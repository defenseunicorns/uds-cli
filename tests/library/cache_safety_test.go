// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachePrivateDirectoriesPublicAPI(t *testing.T) {
	remote := startCacheContractRegistry(t)
	fixture := createLibraryBundle(t)
	config := libraryRegistryConfig(fixture.Config)
	ref := remote.host + "/library/private-cache:v1"
	_, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	config.Options.CacheDir = filepath.Join(t.TempDir(), "cache")
	_, err = bundle.Pull(t.Context(), ref, t.TempDir(), bundle.PullOptions{Config: config, SkipSignatureVerification: true})
	require.NoError(t, err)
	require.NoError(t, filepath.WalkDir(config.Options.CacheDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			assert.Zero(t, info.Mode().Perm()&0o077, "private cache directory %s has mode %o", path, info.Mode().Perm())
		}
		return nil
	}))
}
