// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
)

func TestCopyFixtureIsWritableWithoutChangingSource(t *testing.T) {
	copied := CopyFixture(t, "bundles/02-variables")
	require.NoError(t, os.WriteFile(filepath.Join(copied, "scratch"), []byte("x"), 0o600))
	_, err := os.Stat(TestDataPath("bundles/02-variables/scratch"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCopyFixtureCreatesIndependentWorkspaces(t *testing.T) {
	first := CopyFixture(t, "bundles/02-variables")
	second := CopyFixture(t, "bundles/02-variables")
	secondBundle := filepath.Join(second, "uds-bundle.yaml")
	before, err := os.ReadFile(secondBundle)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(first, "uds-bundle.yaml"), []byte("changed"), 0o600))
	after, err := os.ReadFile(secondBundle)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestFixturePathsRejectTraversal(t *testing.T) {
	for _, path := range []string{"../bundles/02-variables", t.TempDir()} {
		t.Run(path, func(t *testing.T) {
			require.Panics(t, func() { TestDataPath(path) })
			require.Panics(t, func() { CopyFixture(t, path) })
		})
	}
}

func TestPrepareBundleForNamespaceCopiesPackageSourcesAndSetsOverrides(t *testing.T) {
	const namespace = "uds-it-dev-deploy-a1b2c3d4"

	bundleDir := PrepareBundleForNamespace(t, "bundles/03-local-and-remote", namespace)

	var bundle types.UDSBundle
	require.NoError(t, utils.ReadYAMLStrict(filepath.Join(bundleDir, config.BundleYAML), &bundle))
	require.Len(t, bundle.Packages, 2)
	for _, pkg := range bundle.Packages {
		require.Equal(t, namespace, pkg.Namespace)
	}
	require.FileExists(t, filepath.Clean(filepath.Join(bundleDir, bundle.Packages[1].Path, "zarf.yaml")))

	copiedPackage := filepath.Clean(filepath.Join(bundleDir, bundle.Packages[1].Path, "zarf.yaml"))
	require.NoError(t, os.WriteFile(copiedPackage, []byte("changed"), 0o600))
	source, err := os.ReadFile(TestDataPath("packages/podinfo/zarf.yaml"))
	require.NoError(t, err)
	require.NotEqual(t, "changed", string(source))
}

func TestNewRegistryServesOCIRequestsAndClosesAtTestEnd(t *testing.T) {
	var host string
	t.Run("registry lifetime", func(t *testing.T) {
		registry := NewRegistry(t)
		host = registry.Host

		response, err := http.Get("http://" + registry.Host + "/v2/")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.NoError(t, response.Body.Close())
	})

	_, err := http.Get("http://" + host + "/v2/")
	require.Error(t, err)
}
