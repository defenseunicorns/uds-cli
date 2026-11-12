// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
	"github.com/stretchr/testify/require"
)

func TestCreateZarfPkgsUsesBundleArchitecture(t *testing.T) {
	arch := "amd64"
	if runtime.GOARCH == arch {
		arch = "arm64"
	}
	originalArch, originalArgs := config.CLIArch, os.Args
	config.CLIArch = ""
	t.Cleanup(func() { config.CLIArch, os.Args = originalArch, originalArgs })
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "uds-bundle.yaml"), []byte("kind: UDSBundle\nmetadata:\n  name: test\n  architecture: "+arch+"\npackages:\n  - name: app\n    path: .\n    ref: 1.0.0\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zarf.yaml"), []byte("apiVersion: zarf.dev/v1alpha1\nkind: ZarfPackageConfig\nmetadata:\n  name: app\n  version: 1.0.0\ncomponents:\n  - name: files\n    required: true\n    files:\n      - source: data.txt\n        target: /tmp/uds-legacy-test-data.txt\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.txt"), []byte("local fixture"), 0o600))
	b := Bundle{cfg: &types.BundleConfig{CreateOpts: types.BundleCreateOptions{SourceDirectory: dir, BundleFile: "uds-bundle.yaml"}}}
	require.NoError(t, b.CreateZarfPkgs())
	require.FileExists(t, filepath.Join(dir, "zarf-package-app-"+arch+"-1.0.0.tar.zst"))
	require.Equal(t, originalArgs, os.Args)
}
