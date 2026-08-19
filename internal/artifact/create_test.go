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
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCreateStore(t *testing.T) (*oci.Store, string) {
	t.Helper()
	root := t.TempDir()
	store, err := oci.CreateStore(root)
	require.NoError(t, err)
	return store, root
}

func mustTestCreateStore(t *testing.T) *oci.Store {
	t.Helper()
	store, _ := newTestCreateStore(t)
	return store
}

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
		store, storeRoot := newTestCreateStore(t)
		manifests, err := ingestSource(t.Context(), &spec.Package{
			Name: "signed", Source: pkgDir,
			SignatureVerification: &spec.PackageSignatureVerification{PublicKey: "test public key"},
		}, newConfig(t), store, t.TempDir(), iostreams.IOStreams{})
		require.ErrorContains(t, err, "package is not signed")
		assert.Empty(t, manifests)
		entries, readErr := os.ReadDir(filepath.Join(storeRoot, "blobs", "sha256"))
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
		}, newConfig(t), mustTestCreateStore(t), t.TempDir(), streams)
		require.NoError(t, err)
		require.Len(t, manifests, 1)
		assert.Equal(t, "unsigned", manifests[0].Annotations[oci.AnnotationPackageName])
		assert.Equal(t, pkgDir, manifests[0].Annotations[oci.AnnotationPackageSource])
		assert.Equal(t, "unsigned", manifests[0].Annotations[ocispec.AnnotationRefName])
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
			_, err := ingestSource(t.Context(), tt.pkg, tt.config, mustTestCreateStore(t), t.TempDir(), iostreams.IOStreams{})
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestAnnotatePackageDescriptorUsesBundlePackageNameForOCISources(t *testing.T) {
	desc := annotatePackageDescriptor(ocispec.Descriptor{Annotations: map[string]string{
		oci.AnnotationPackageVerification: oci.AnnotationPackageVerificationVerified,
	}}, &spec.Package{Name: "mypkg", Source: "oci://example.com/pkg:v1"})

	assert.Equal(t, "mypkg", desc.Annotations[oci.AnnotationPackageName])
	assert.Equal(t, "oci://example.com/pkg:v1", desc.Annotations[oci.AnnotationPackageSource])
	assert.Equal(t, "mypkg", desc.Annotations[ocispec.AnnotationRefName])
	assert.NotContains(t, desc.Annotations, oci.AnnotationPackageVerification)
}

func TestAnnotatePackageVerification(t *testing.T) {
	manifests := []ocispec.Descriptor{{}, {Annotations: map[string]string{"existing": "value"}}}
	annotatePackageVerification(manifests, true)

	for _, manifest := range manifests {
		assert.Equal(t, oci.AnnotationPackageVerificationVerified, manifest.Annotations[oci.AnnotationPackageVerification])
	}

	manifests = []ocispec.Descriptor{{}}
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
