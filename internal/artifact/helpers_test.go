// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

const (
	BundleFileName         = bundleinternal.BundleFileName
	BundleDefaultsFileName = bundleinternal.BundleDefaultsFileName
)

// ociIndex aliases the OCI index model for artifact tests.
type ociIndex = oci.OciIndex

// ociManifest aliases the OCI manifest model for artifact tests.
type ociManifest = oci.OciManifest

// testDescriptor models OCI content descriptors assembled by artifact tests.
type testDescriptor struct {
	MediaType   string            `json:"mediaType,omitempty"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// testImageManifest models image manifests assembled by artifact tests.
type testImageManifest struct {
	SchemaVersion int              `json:"schemaVersion"`
	Config        testDescriptor   `json:"config"`
	Layers        []testDescriptor `json:"layers"`
}

// buildBundleArtifact creates a test bundle artifact without defaults.
func buildBundleArtifact(t *testing.T, bundleHCL string, valuesFiles map[string][]string, pkgSources []string) string {
	t.Helper()
	return buildBundleArtifactInner(t, bundleHCL, "", valuesFiles, pkgSources, BundleFileName, "zarf.yaml")
}

// buildBundleArtifactWithDefaults creates a test artifact containing defaults HCL.
func buildBundleArtifactWithDefaults(t *testing.T, bundleHCL, defaultsHCL string, valuesFiles map[string][]string, pkgSources []string) string {
	t.Helper()
	return buildBundleArtifactInner(t, bundleHCL, defaultsHCL, valuesFiles, pkgSources, BundleFileName, "zarf.yaml")
}

// buildBundleArtifactWithTitles creates a test artifact with custom layer titles.
func buildBundleArtifactWithTitles(t *testing.T, bundleHCL string, valuesFiles map[string][]string, pkgSources []string, bundleTitle, packageLayerTitle string) string {
	t.Helper()
	return buildBundleArtifactInner(t, bundleHCL, "", valuesFiles, pkgSources, bundleTitle, packageLayerTitle)
}

// buildBundleArtifactInner assembles the OCI layout shared by artifact test builders.
func buildBundleArtifactInner(t *testing.T, bundleHCL, defaultsHCL string, valuesFiles map[string][]string, pkgSources []string, bundleTitle, packageLayerTitle string) string {
	t.Helper()
	root := t.TempDir()
	ociDir := filepath.Join(root, "oci")
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))
	require.NoError(t, os.WriteFile(filepath.Join(ociDir, "oci-layout"), []byte("{\"imageLayoutVersion\":\"1.0.0\"}\n"), tmpFilePerm))

	writeBlob := func(data []byte) string {
		sum := sha256.Sum256(data)
		hexDigest := hex.EncodeToString(sum[:])
		require.NoError(t, os.WriteFile(filepath.Join(blobDir, hexDigest), data, tmpFilePerm))
		return "sha256:" + hexDigest
	}
	emptyConfig := []byte("{}")
	emptyDigest := writeBlob(emptyConfig)
	layers := []testDescriptor{{
		MediaType: oci.MediaTypeBundleHCL, Digest: writeBlob([]byte(bundleHCL)), Size: int64(len(bundleHCL)),
		Annotations: map[string]string{ocispec.AnnotationTitle: bundleTitle},
	}}
	if defaultsHCL != "" {
		layers = append(layers, testDescriptor{
			MediaType: oci.MediaTypeBundleHCL, Digest: writeBlob([]byte(defaultsHCL)), Size: int64(len(defaultsHCL)),
			Annotations: map[string]string{ocispec.AnnotationTitle: BundleDefaultsFileName},
		})
	}
	for packageName, files := range valuesFiles {
		for i, value := range files {
			layers = append(layers, testDescriptor{
				MediaType: oci.MediaTypeBundleValuesYAML, Digest: writeBlob([]byte(value)), Size: int64(len(value)),
				Annotations: map[string]string{ocispec.AnnotationTitle: fmt.Sprintf("values/%s/%d.yaml", packageName, i)},
			})
		}
	}
	definition := testImageManifest{SchemaVersion: 2, Config: testDescriptor{Digest: emptyDigest, Size: int64(len(emptyConfig))}, Layers: layers}
	definitionData, err := json.Marshal(definition)
	require.NoError(t, err)
	manifests := []oci.OciManifest{{
		MediaType: ocispec.MediaTypeImageManifest, ArtifactType: oci.MediaTypeBundleDefinition,
		Digest: writeBlob(definitionData), Size: int64(len(definitionData)),
	}}
	for _, source := range pkgSources {
		packageData := []byte("fake package: " + source)
		packageManifest := testImageManifest{
			SchemaVersion: 2,
			Config:        testDescriptor{Digest: emptyDigest, Size: int64(len(emptyConfig))},
			Layers: []testDescriptor{{
				MediaType: oci.MediaTypeZarfLayer, Digest: writeBlob(packageData), Size: int64(len(packageData)),
				Annotations: map[string]string{ocispec.AnnotationTitle: packageLayerTitle},
			}},
		}
		packageManifestData, err := json.Marshal(packageManifest)
		require.NoError(t, err)
		refName := source
		if oci.IsOCIReference(source) {
			refName = oci.TrimScheme(source)
		}
		manifests = append(manifests, oci.OciManifest{
			MediaType: ocispec.MediaTypeImageManifest, Digest: writeBlob(packageManifestData), Size: int64(len(packageManifestData)),
			Annotations: map[string]string{"org.opencontainers.image.ref.name": refName},
		})
	}
	index := oci.OciIndex{
		SchemaVersion: 2, MediaType: ocispec.MediaTypeImageIndex, ArtifactType: oci.MediaTypeBundle,
		Manifests: manifests, Annotations: map[string]string{oci.AnnotationBundleArchitecture: "amd64"},
	}
	indexData, err := json.MarshalIndent(index, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(ociDir, "index.json"), append(indexData, '\n'), tmpFilePerm))
	outPath := filepath.Join(t.TempDir(), "bundle.tar.zst")
	require.NoError(t, WriteTarZst(t.Context(), iostreams.IOStreams{}, outPath, root))
	return outPath
}
