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
	"github.com/defenseunicorns/uds-cli/internal/filesystem"
	"github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	godigest "github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

const (
	BundleFileName         = bundleinternal.BundleFileName
	BundleDefaultsFileName = bundleinternal.BundleDefaultsFileName
)

// buildBundleArtifact creates a test bundle artifact without defaults.
func buildBundleArtifact(t *testing.T, bundleHCL string, valuesFiles map[string][]string, pkgs []spec.Package) string {
	t.Helper()
	return buildBundleArtifactInner(t, bundleHCL, "", valuesFiles, pkgs, BundleFileName, "zarf.yaml")
}

// buildBundleArtifactWithDefaults creates a test artifact containing defaults HCL.
func buildBundleArtifactWithDefaults(t *testing.T, bundleHCL, defaultsHCL string, valuesFiles map[string][]string, pkgs []spec.Package) string {
	t.Helper()
	return buildBundleArtifactInner(t, bundleHCL, defaultsHCL, valuesFiles, pkgs, BundleFileName, "zarf.yaml")
}

// buildBundleArtifactWithTitles creates a test artifact with custom layer titles.
func buildBundleArtifactWithTitles(t *testing.T, bundleHCL string, valuesFiles map[string][]string, pkgs []spec.Package, bundleTitle, packageLayerTitle string) string {
	t.Helper()
	return buildBundleArtifactInner(t, bundleHCL, "", valuesFiles, pkgs, bundleTitle, packageLayerTitle)
}

// buildBundleArtifactInner assembles the OCI layout shared by artifact test builders.
func buildBundleArtifactInner(t *testing.T, bundleHCL, defaultsHCL string, valuesFiles map[string][]string, pkgs []spec.Package, bundleTitle, packageLayerTitle string) string {
	t.Helper()
	root := t.TempDir()
	ociDir := filepath.Join(root, "oci")
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, filesystem.PrivateDirectoryMode))
	require.NoError(t, os.WriteFile(filepath.Join(ociDir, "oci-layout"), []byte("{\"imageLayoutVersion\":\"1.0.0\"}\n"), filesystem.PrivateFileMode))

	writeBlob := func(data []byte) godigest.Digest {
		sum := sha256.Sum256(data)
		hexDigest := hex.EncodeToString(sum[:])
		require.NoError(t, os.WriteFile(filepath.Join(blobDir, hexDigest), data, filesystem.PrivateFileMode))
		return godigest.NewDigestFromEncoded(godigest.SHA256, hexDigest)
	}
	emptyConfig := []byte("{}")
	emptyDigest := writeBlob(emptyConfig)
	layers := []ocispec.Descriptor{{
		MediaType: oci.MediaTypeBundleHCL, Digest: writeBlob([]byte(bundleHCL)), Size: int64(len(bundleHCL)),
		Annotations: map[string]string{ocispec.AnnotationTitle: bundleTitle},
	}}
	if defaultsHCL != "" {
		layers = append(layers, ocispec.Descriptor{
			MediaType: oci.MediaTypeBundleHCL, Digest: writeBlob([]byte(defaultsHCL)), Size: int64(len(defaultsHCL)),
			Annotations: map[string]string{ocispec.AnnotationTitle: BundleDefaultsFileName},
		})
	}
	for packageName, files := range valuesFiles {
		for i, value := range files {
			layers = append(layers, ocispec.Descriptor{
				MediaType: oci.MediaTypeBundleValuesYAML, Digest: writeBlob([]byte(value)), Size: int64(len(value)),
				Annotations: map[string]string{ocispec.AnnotationTitle: fmt.Sprintf("values/%s/%d.yaml", packageName, i)},
			})
		}
	}
	definition := ocispec.Manifest{Versioned: specs.Versioned{SchemaVersion: 2}, Config: ocispec.Descriptor{Digest: emptyDigest, Size: int64(len(emptyConfig))}, Layers: layers}
	definitionData, err := json.Marshal(definition)
	require.NoError(t, err)
	manifests := []ocispec.Descriptor{{
		MediaType: ocispec.MediaTypeImageManifest, ArtifactType: oci.MediaTypeBundleDefinition,
		Digest: writeBlob(definitionData), Size: int64(len(definitionData)),
	}}
	for _, pkg := range pkgs {
		packageData := fmt.Appendf(nil, "metadata:\n  name: %s\nsource: %s\n", pkg.Name, pkg.Source)
		packageManifest := ocispec.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			Config:    ocispec.Descriptor{Digest: emptyDigest, Size: int64(len(emptyConfig))},
			Layers: []ocispec.Descriptor{{
				MediaType: oci.MediaTypeZarfLayer, Digest: writeBlob(packageData), Size: int64(len(packageData)),
				Annotations: map[string]string{ocispec.AnnotationTitle: packageLayerTitle},
			}},
		}
		packageManifestData, err := json.Marshal(packageManifest)
		require.NoError(t, err)
		manifests = append(manifests, ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageManifest, Digest: writeBlob(packageManifestData), Size: int64(len(packageManifestData)),
			Annotations: map[string]string{
				oci.AnnotationPackageName:   pkg.Name,
				oci.AnnotationPackageSource: pkg.Source,
				ocispec.AnnotationRefName:   pkg.Name,
			},
		})
	}
	index := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageIndex, ArtifactType: oci.MediaTypeBundle,
		Manifests: manifests, Annotations: map[string]string{oci.AnnotationBundleArchitecture: "amd64"},
	}
	indexData, err := json.MarshalIndent(index, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(ociDir, "index.json"), append(indexData, '\n'), filesystem.PrivateFileMode))
	outPath := filepath.Join(t.TempDir(), "bundle.tar.zst")
	require.NoError(t, WriteTarZst(t.Context(), iostreams.IOStreams{}, outPath, root))
	return outPath
}
