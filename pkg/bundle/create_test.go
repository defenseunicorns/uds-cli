// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeMinimalZarfPackage(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, tempDirPerm))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zarf.yaml"), []byte("kind: ZarfPackageConfig\nmetadata:\n  name: test\n  version: 0.0.1\n  aggregateChecksum: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\ncomponents: []\n"), tmpFilePerm))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "checksums.txt"), nil, tmpFilePerm))
}

func TestCreate_BuildsTarZstWithExpectedLayout(t *testing.T) {
	dir := t.TempDir()

	localLayout := filepath.Join(dir, "localpkg")
	writeMinimalZarfPackage(t, localLayout)

	valuesDir := filepath.Join(dir, "values")
	require.NoError(t, os.MkdirAll(valuesDir, tempDirPerm))
	require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "a.yaml"), []byte("a: 1\n"), tmpFilePerm))

	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name    = "test"
  version = "0.0.1"
}

package "pkg1" {
  source = "localpkg"
  signature_verification { verify = false }
  values_files = ["values/a.yaml"]
}
`), tmpFilePerm))

	_, err := Create(t.Context(), bundleFile, CreateOptions{
		Config:  newTestConfig(),
		Signing: SigningOptions{Mode: SigningModeUnsigned},
		Streams: iostreams.New(nil, nil, io.Discard),
	})
	require.NoError(t, err)

	outPath := filepath.Join(dir, "uds-bundle-test-"+runtime.GOARCH+"-0.0.1.tar.zst")
	entries := readTarZstEntries(t, outPath)

	require.Contains(t, entries, "oci/oci-layout")
	require.Contains(t, entries, "oci/index.json")
	require.True(t, bundleDefinitionContainsLayerTitle(t, entries, "bundle.uds.hcl"))
	require.True(t, bundleDefinitionContainsLayerTitle(t, entries, "values/pkg1/0.yaml"))

	// Parse the OCI index and locate the bundle definition manifest by artifactType.
	var idx struct {
		Manifests []struct {
			Digest       string `json:"digest"`
			ArtifactType string `json:"artifactType"`
		} `json:"manifests"`
	}
	require.NoError(t, json.Unmarshal(entries["oci/index.json"], &idx))
	require.Len(t, idx.Manifests, 2) // config manifest + package manifest

	var cfgDigest string
	for _, m := range idx.Manifests {
		require.Contains(t, entries, "oci/blobs/sha256/"+strings.TrimPrefix(m.Digest, "sha256:"))
		if m.ArtifactType == udsoci.MediaTypeBundleDefinition {
			cfgDigest = m.Digest
		}
	}
	require.NotEmpty(t, cfgDigest, "bundle definition manifest not found in OCI index")

	// Parse the config manifest and verify layer titles and content.
	cfgBlob := entries["oci/blobs/sha256/"+strings.TrimPrefix(cfgDigest, "sha256:")]
	require.NotNil(t, cfgBlob)

	var cfgManifest struct {
		ArtifactType string `json:"artifactType"`
		Layers       []struct {
			Digest      string            `json:"digest"`
			Annotations map[string]string `json:"annotations"`
		} `json:"layers"`
	}
	require.NoError(t, json.Unmarshal(cfgBlob, &cfgManifest))
	require.Equal(t, udsoci.MediaTypeBundleDefinition, cfgManifest.ArtifactType)
	require.Len(t, cfgManifest.Layers, 2) // HCL + 1 values file

	titles := make([]string, len(cfgManifest.Layers))
	for i, l := range cfgManifest.Layers {
		titles[i] = l.Annotations["org.opencontainers.image.title"]
	}
	require.Contains(t, titles, "bundle.uds.hcl")
	require.Contains(t, titles, "values/pkg1/0.yaml")

	// Verify the values file content is preserved in the blob.
	for _, l := range cfgManifest.Layers {
		if l.Annotations["org.opencontainers.image.title"] == "values/pkg1/0.yaml" {
			blob := entries["oci/blobs/sha256/"+strings.TrimPrefix(l.Digest, "sha256:")]
			require.Equal(t, "a: 1\n", string(blob))
		}
	}
}

func TestCreate_SharedValuesFileDeduplicatedInOCIStore(t *testing.T) {
	dir := t.TempDir()

	writeMinimalZarfPackage(t, filepath.Join(dir, "pkg1"))
	writeMinimalZarfPackage(t, filepath.Join(dir, "pkg2"))

	valuesDir := filepath.Join(dir, "values")
	require.NoError(t, os.MkdirAll(valuesDir, tempDirPerm))
	require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "shared.yaml"), []byte("shared: true\n"), tmpFilePerm))

	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name    = "shared-values-test"
  version = "0.0.1"
}

package "pkg1" {
  source = "pkg1"
  signature_verification { verify = false }
  values_files = ["values/shared.yaml"]
}

package "pkg2" {
  source = "pkg2"
  signature_verification { verify = false }
  values_files = ["values/shared.yaml"]
}
`), tmpFilePerm))

	_, err := Create(t.Context(), bundleFile, CreateOptions{
		Config:  newTestConfig(),
		Signing: SigningOptions{Mode: SigningModeUnsigned},
		Streams: iostreams.New(nil, nil, io.Discard),
	})
	require.NoError(t, err)

	outPath := filepath.Join(dir, "uds-bundle-shared-values-test-"+runtime.GOARCH+"-0.0.1.tar.zst")
	entries := readTarZstEntries(t, outPath)

	// Locate the bundle definition manifest.
	var idx struct {
		Manifests []struct {
			Digest       string            `json:"digest"`
			ArtifactType string            `json:"artifactType"`
			Annotations  map[string]string `json:"annotations"`
		} `json:"manifests"`
	}
	require.NoError(t, json.Unmarshal(entries["oci/index.json"], &idx))
	require.Len(t, idx.Manifests, 3) // config manifest + two package manifests

	var packageRefs []string
	var packageNames []string
	var packageSources []string
	var definitionCount int
	for _, m := range idx.Manifests {
		if m.ArtifactType == udsoci.MediaTypeBundleDefinition {
			definitionCount++
			continue
		}
		packageRefs = append(packageRefs, m.Annotations[ocispec.AnnotationRefName])
		packageNames = append(packageNames, m.Annotations[udsoci.AnnotationPackageName])
		packageSources = append(packageSources, m.Annotations[udsoci.AnnotationPackageSource])
	}
	require.Equal(t, 1, definitionCount)
	assert.ElementsMatch(t, []string{"pkg1", "pkg2"}, packageRefs)
	assert.ElementsMatch(t, []string{"pkg1", "pkg2"}, packageNames)
	assert.ElementsMatch(t, []string{"pkg1", "pkg2"}, packageSources)

	var defManifestBytes []byte
	for _, m := range idx.Manifests {
		if m.ArtifactType == udsoci.MediaTypeBundleDefinition {
			hex := strings.TrimPrefix(m.Digest, "sha256:")
			defManifestBytes = entries["oci/blobs/sha256/"+hex]
			break
		}
	}
	require.NotNil(t, defManifestBytes, "bundle definition manifest not found")

	var defManifest struct {
		Layers []struct {
			Digest      string            `json:"digest"`
			Annotations map[string]string `json:"annotations"`
		} `json:"layers"`
	}
	require.NoError(t, json.Unmarshal(defManifestBytes, &defManifest))

	// Each package must have its own titled layer entry.
	layersByTitle := make(map[string]string)
	for _, l := range defManifest.Layers {
		title := l.Annotations["org.opencontainers.image.title"]
		layersByTitle[title] = l.Digest
	}
	require.Contains(t, layersByTitle, "values/pkg1/0.yaml")
	require.Contains(t, layersByTitle, "values/pkg2/0.yaml")

	// Both layer entries must point to the same blob digest.
	require.Equal(t, layersByTitle["values/pkg1/0.yaml"], layersByTitle["values/pkg2/0.yaml"],
		"shared values file content should map to a single deduplicated blob")

	// Only one blob should exist in the OCI store for that digest.
	blobPath := "oci/blobs/sha256/" + strings.TrimPrefix(layersByTitle["values/pkg1/0.yaml"], "sha256:")
	require.Contains(t, entries, blobPath)
}

func TestCreate_DefaultsHCLIncludedWhenPresent(t *testing.T) {
	dir := t.TempDir()

	writeMinimalZarfPackage(t, filepath.Join(dir, "localpkg"))

	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name    = "defaults-test"
  version = "0.0.1"
}

package "pkg1" {
  source = "localpkg"
  signature_verification { verify = false }
}
`), tmpFilePerm))

	defaultsContent := []byte(`variables = {
  a = "a-default-value"
  b = "b-default-value"
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, bundleDefaultsFileName), defaultsContent, tmpFilePerm))

	_, err := Create(t.Context(), bundleFile, CreateOptions{
		Config:  newTestConfig(),
		Signing: SigningOptions{Mode: SigningModeUnsigned},
		Streams: iostreams.New(nil, nil, io.Discard),
	})
	require.NoError(t, err)

	outPath := filepath.Join(dir, "uds-bundle-defaults-test-"+runtime.GOARCH+"-0.0.1.tar.zst")
	entries := readTarZstEntries(t, outPath)

	require.True(t, bundleDefinitionContainsLayerTitle(t, entries, "bundle.uds.hcl"))
	require.True(t, bundleDefinitionContainsLayerTitle(t, entries, bundleDefaultsFileName))

	// Verify defaults.uds.hcl content is preserved in the blob.
	var idx struct {
		Manifests []struct {
			Digest       string `json:"digest"`
			ArtifactType string `json:"artifactType"`
		} `json:"manifests"`
	}
	require.NoError(t, json.Unmarshal(entries["oci/index.json"], &idx))

	var defManifestBytes []byte
	for _, m := range idx.Manifests {
		if m.ArtifactType == udsoci.MediaTypeBundleDefinition {
			hex := strings.TrimPrefix(m.Digest, "sha256:")
			defManifestBytes = entries["oci/blobs/sha256/"+hex]
			break
		}
	}
	require.NotNil(t, defManifestBytes)

	var defManifest struct {
		Layers []struct {
			Digest      string            `json:"digest"`
			Annotations map[string]string `json:"annotations"`
		} `json:"layers"`
	}
	require.NoError(t, json.Unmarshal(defManifestBytes, &defManifest))

	for _, l := range defManifest.Layers {
		if l.Annotations["org.opencontainers.image.title"] == bundleDefaultsFileName {
			blob := entries["oci/blobs/sha256/"+strings.TrimPrefix(l.Digest, "sha256:")]
			require.Equal(t, string(defaultsContent), string(blob))
		}
	}
}

func TestCreate_NoDefaultsHCL_NotIncluded(t *testing.T) {
	dir := t.TempDir()

	writeMinimalZarfPackage(t, filepath.Join(dir, "localpkg"))

	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name    = "no-defaults-test"
  version = "0.0.1"
}

package "pkg1" {
  source = "localpkg"
  signature_verification { verify = false }
}
`), tmpFilePerm))

	_, err := Create(t.Context(), bundleFile, CreateOptions{
		Config:  newTestConfig(),
		Signing: SigningOptions{Mode: SigningModeUnsigned},
		Streams: iostreams.New(nil, nil, io.Discard),
	})
	require.NoError(t, err)

	outPath := filepath.Join(dir, "uds-bundle-no-defaults-test-"+runtime.GOARCH+"-0.0.1.tar.zst")
	entries := readTarZstEntries(t, outPath)

	require.True(t, bundleDefinitionContainsLayerTitle(t, entries, "bundle.uds.hcl"))
	require.False(t, bundleDefinitionContainsLayerTitle(t, entries, bundleDefaultsFileName),
		"defaults.uds.hcl should not be in the bundle when the file does not exist")
}

func TestCreate_BundleIndexIsDeterministicAndSelfIdentifying(t *testing.T) {
	t.Parallel()

	// One source directory, two create runs: identical inputs must produce a
	// byte-identical bundle index (the fixture package content is random per
	// fixture, so the source is built once).
	dir := t.TempDir()
	writeMinimalZarfPackage(t, filepath.Join(dir, "localpkg"))
	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "determinism"
  version = "0.1.0"
}
package "pkg1" {
  source = "localpkg"
  signature_verification { verify = false }
}
`), tmpFilePerm))

	createOnce := func() []byte {
		result, err := Create(t.Context(), bundleFile, CreateOptions{
			Config:  newTestConfigWithArch("amd64"),
			Signing: SigningOptions{Mode: SigningModeUnsigned},
			Streams: iostreams.New(nil, nil, io.Discard),
		})
		require.NoError(t, err)
		entries := readTarZstEntries(t, result.OutputPath)
		idxBytes, ok := entries["oci/index.json"]
		require.True(t, ok)
		require.NoError(t, os.Remove(result.OutputPath))
		return idxBytes
	}

	first := createOnce()
	second := createOnce()
	assert.Equal(t, first, second, "identical inputs must produce byte-identical bundle indexes")

	var idx ocispec.Index
	require.NoError(t, json.Unmarshal(first, &idx))
	assert.Equal(t, udsoci.MediaTypeBundle, idx.ArtifactType, "bundle index must self-identify via artifactType")
	assert.Equal(t, "amd64", idx.Annotations[udsoci.AnnotationBundleArchitecture], "bundle index must record its architecture")
	for i, m := range idx.Manifests {
		assert.Nil(t, m.Platform, "bundle index entries must not carry a platform")
		if i > 0 {
			assert.LessOrEqual(t, idx.Manifests[i-1].Digest, m.Digest, "bundle index entries must be sorted by digest")
		}
	}
}
