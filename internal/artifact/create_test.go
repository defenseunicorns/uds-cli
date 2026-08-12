// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"os"
	"path/filepath"
	"testing"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIngestSourceVerificationBoundary(t *testing.T) {
	newConfig := func(t *testing.T) *bundleinternal.UDSBundleConfig {
		t.Helper()
		config := validValidationConfig()
		config.Options.Architecture = "amd64"
		config.Options.TmpDir = t.TempDir()
		return config
	}

	t.Run("verification failure never ingests the real package", func(t *testing.T) {
		pkgDir := t.TempDir()
		writeValidUnsignedPackage(t, pkgDir)
		blobDir := t.TempDir()
		manifests, err := ingestSource(t.Context(), &spec.Package{
			Name: "signed", Source: pkgDir,
			SignatureVerification: &spec.PackageSignatureVerification{PublicKey: "test public key"},
		}, newConfig(t), blobDir, t.TempDir(), iostreams.IOStreams{})
		require.ErrorContains(t, err, "package is not signed")
		assert.Empty(t, manifests)
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

		manifests, err := ingestSource(t.Context(), &spec.Package{
			Name: "unsigned", Source: pkgDir,
			SignatureVerification: &spec.PackageSignatureVerification{Verify: &verify},
		}, newConfig(t), t.TempDir(), t.TempDir(), streams)
		require.NoError(t, err)
		require.Len(t, manifests, 1)
		assert.Contains(t, out.String()+errOut.String(), "unverified package")
	})
}

func TestIngestSourceRejectsNilInputs(t *testing.T) {
	config := validValidationConfig()
	tests := []struct {
		name   string
		pkg    *spec.Package
		config *bundleinternal.UDSBundleConfig
		want   string
	}{
		{name: "package", config: config, want: "package must not be nil"},
		{name: "configuration", pkg: &spec.Package{}, want: "config must not be nil"},
		{name: "configuration options", pkg: &spec.Package{}, config: &bundleinternal.UDSBundleConfig{}, want: "config.Options must not be nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ingestSource(t.Context(), tt.pkg, tt.config, t.TempDir(), t.TempDir(), iostreams.IOStreams{})
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestAnnotatePackageVerification(t *testing.T) {
	manifests := []oci.OciManifest{{}, {Annotations: map[string]string{"existing": "value"}}}
	annotatePackageVerification(manifests, true)

	for _, manifest := range manifests {
		assert.Equal(t, oci.AnnotationPackageVerificationVerified, manifest.Annotations[oci.AnnotationPackageVerification])
	}

	manifests = []oci.OciManifest{{}}
	annotatePackageVerification(manifests, false)
	assert.Empty(t, manifests[0].Annotations)
}

func writeValidUnsignedPackage(t *testing.T, dir string) {
	t.Helper()
	const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	zarfYAML := "kind: ZarfPackageConfig\nmetadata:\n  name: test\n  version: 1.0.0\n  aggregateChecksum: " + emptySHA256 + "\ncomponents: []\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zarf.yaml"), []byte(zarfYAML), tmpFilePerm))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "checksums.txt"), nil, tmpFilePerm))
}
