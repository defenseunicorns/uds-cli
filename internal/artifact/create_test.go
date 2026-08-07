// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalCreatorCreatePackageVerificationBoundary(t *testing.T) {
	newOptions := func(t *testing.T, blobDir string, streams iostreams.IOStreams) CreatePackageOptions {
		t.Helper()
		config := validValidationConfig()
		config.Options.Architecture = "amd64"
		config.Options.TmpDir = t.TempDir()
		return CreatePackageOptions{Config: config, BlobDir: blobDir, BundleDir: t.TempDir(), Streams: streams}
	}

	t.Run("verification failure never ingests the real package", func(t *testing.T) {
		pkgDir := t.TempDir()
		writeValidUnsignedPackage(t, pkgDir)
		blobDir := t.TempDir()
		creator := newLocalCreator("amd64")
		err := creator.CreatePackage(t.Context(), &spec.Package{
			Name: "signed", Source: pkgDir,
			SignatureVerification: &spec.PackageSignatureVerification{PublicKey: "test public key"},
		}, newOptions(t, blobDir, iostreams.IOStreams{}))
		require.ErrorContains(t, err, "package is not signed")
		assert.Empty(t, creator.manifests)
		entries, readErr := os.ReadDir(blobDir)
		require.NoError(t, readErr)
		assert.Empty(t, entries)
	})

	t.Run("explicit bypass ingests a real package and warns", func(t *testing.T) {
		verify := false
		pkgDir := t.TempDir()
		writeValidUnsignedPackage(t, pkgDir)
		streams, _, out, errOut := iostreams.NewTestIOStreams()
		streams = logger.Bind(streams, "info")
		creator := newLocalCreator("amd64")

		err := creator.CreatePackage(t.Context(), &spec.Package{
			Name: "unsigned", Source: pkgDir,
			SignatureVerification: &spec.PackageSignatureVerification{Verify: &verify},
		}, newOptions(t, t.TempDir(), streams))
		require.NoError(t, err)
		require.Len(t, creator.manifests, 1)
		assert.Contains(t, out.String()+errOut.String(), "unverified package")
	})

	t.Run("nil package is rejected", func(t *testing.T) {
		err := newLocalCreator("amd64").CreatePackage(t.Context(), nil, newOptions(t, t.TempDir(), iostreams.IOStreams{}))
		require.ErrorContains(t, err, "package is required")
	})
}

func writeValidUnsignedPackage(t *testing.T, dir string) {
	t.Helper()
	const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	zarfYAML := "kind: ZarfPackageConfig\nmetadata:\n  name: test\n  version: 1.0.0\n  aggregateChecksum: " + emptySHA256 + "\ncomponents: []\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zarf.yaml"), []byte(zarfYAML), tmpFilePerm))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "checksums.txt"), nil, tmpFilePerm))
}
