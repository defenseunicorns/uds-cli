// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingBatchFetcher struct {
	archive    archiveContentFetcher
	fetches    int
	batchReads int
}

func (f *countingBatchFetcher) Fetch(ctx context.Context, descriptor ocispec.Descriptor) (io.ReadCloser, error) {
	f.fetches++
	return f.archive.Fetch(ctx, descriptor)
}

func (f *countingBatchFetcher) FetchBatch(ctx context.Context, descriptors []ocispec.Descriptor, visit func(ocispec.Descriptor, []byte) error) error {
	f.batchReads++
	return f.archive.FetchBatch(ctx, descriptors, visit)
}

func TestLocalArchiveMetadataSourceReadsSelectedMetadata(t *testing.T) {
	artifactPath := buildBundleArtifact(t, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = "test-bundle"
  version = "0.1.0"
}
package "selected" {
  source = "selected"
}
package "unselected" {
  source = "unselected"
}
`, nil, []spec.Package{
		{Name: "selected", Source: "selected"},
		{Name: "unselected", Source: "unselected"},
	})

	local, err := OpenLocalArchiveMetadataSource(t.Context(), artifactPath)
	require.NoError(t, err)
	source := &MetadataSource{IndexBytes: local.Index, ArtifactDigest: local.ArtifactDigest, Fetcher: local.Fetcher}
	metadata, err := ReadBundleDefinition(t.Context(), source, iostreams.IOStreams{})
	require.NoError(t, err)
	assert.Equal(t, "test-bundle", metadata.Bundle.Metadata.Name)

	archiveFetcher, ok := local.Fetcher.(archiveContentFetcher)
	require.True(t, ok)
	countingFetcher := &countingBatchFetcher{archive: archiveFetcher}
	source.Fetcher = countingFetcher
	signatures, err := ReadPackageSignatures(t.Context(), source, metadata.Bundle)
	require.NoError(t, err)
	assert.Len(t, signatures, 2)
	assert.Equal(t, 2, countingFetcher.batchReads, "package manifests and zarf.yaml layers should each require one archive walk")
	assert.Zero(t, countingFetcher.fetches)

	zarfNames, err := ReadZarfPackageNames(t.Context(), source, metadata.Bundle, "selected")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"selected": "selected"}, zarfNames)
	assert.NotEmpty(t, source.IndexBytes)
	assert.False(t, local.SignatureFound)
}

func TestReadPackageSignaturesPreservesOrderedErrorsWithBatchReads(t *testing.T) {
	artifactPath := buildBundleArtifactWithZarfYAML(t, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = "invalid-metadata"
  version = "0.1.0"
}
package "malformed" {
  source = "unused"
}
package "later-malformed" {
  source = "unused"
}
`, []spec.Package{
		{Name: "malformed", Source: "unused"},
		{Name: "later-malformed", Source: "unused"},
	}, map[string][]byte{
		"malformed":       []byte("metadata:\n  name: [\n"),
		"later-malformed": []byte("metadata:\n  name: {\n"),
	})

	local, err := OpenLocalArchiveMetadataSource(t.Context(), artifactPath)
	require.NoError(t, err)
	source := &MetadataSource{IndexBytes: local.Index, ArtifactDigest: local.ArtifactDigest, Fetcher: local.Fetcher}
	metadata, err := ReadBundleDefinition(t.Context(), source, iostreams.IOStreams{})
	require.NoError(t, err)

	archiveFetcher, ok := local.Fetcher.(archiveContentFetcher)
	require.True(t, ok)
	countingFetcher := &countingBatchFetcher{archive: archiveFetcher}
	source.Fetcher = countingFetcher
	_, err = ReadPackageSignatures(t.Context(), source, metadata.Bundle)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrParsingZarfYAML)
	var target InspectingPackageSignatureError
	require.ErrorAs(t, err, &target)
	assert.Equal(t, "malformed", target.Package)
	assert.Equal(t, 2, countingFetcher.batchReads)
	assert.Zero(t, countingFetcher.fetches)
}

func TestLocalArchiveMetadataUsesZarfMultiDocParsingAndMigrations(t *testing.T) {
	zarfYAML := []byte(`apiVersion: unsupported.example/v1
metadata:
  name: ignored
---
apiVersion: zarf.dev/v1alpha1
metadata:
  name: migrated-package
build:
  signed: true
components:
  - name: example
    scripts:
      before:
        - echo migrated
`)
	artifactPath := buildBundleArtifactWithZarfYAML(t, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = "zarf-metadata"
  version = "0.1.0"
}
package "bundle-label" {
  source = "source-must-not-be-read"
}
`, []spec.Package{{Name: "bundle-label", Source: "source-must-not-be-read"}}, map[string][]byte{
		"bundle-label": zarfYAML,
	})

	inspected, err := Inspect(t.Context(), InspectOptions{Source: artifactPath, Streams: iostreams.IOStreams{}})
	require.NoError(t, err)
	require.Len(t, inspected.Packages, 1)
	assert.Equal(t, PackageSigningStatusSigned, inspected.PackageSignatures["bundle-label"].Signed)

	local, err := OpenLocalArchiveMetadataSource(t.Context(), artifactPath)
	require.NoError(t, err)
	var index ocispec.Index
	require.NoError(t, json.Unmarshal(local.Index, &index))
	entry, err := findPackageManifest(index, spec.Package{Name: "bundle-label", Source: "source-must-not-be-read"})
	require.NoError(t, err)

	pkg, found, err := fetchZarfPackage(t.Context(), "bundle-label", *entry, local.Fetcher)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "migrated-package", pkg.Metadata.Name)
	require.NotNil(t, pkg.Build.Signed)
	assert.True(t, *pkg.Build.Signed)
	require.Len(t, pkg.Components, 1)
	require.Len(t, pkg.Components[0].Actions.OnDeploy.Before, 1)
	assert.Equal(t, "echo migrated", pkg.Components[0].Actions.OnDeploy.Before[0].Cmd)
}
