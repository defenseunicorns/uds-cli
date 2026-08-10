// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/mholt/archives"
	"github.com/stretchr/testify/require"
	oras "oras.land/oras-go/v2"
)

const (
	tempDirPerm fs.FileMode = 0o700
	tmpFilePerm fs.FileMode = 0o600
)

// newTestConfig returns bundle configuration for the runtime architecture.
func newTestConfig() *UDSBundleConfig {
	return newTestConfigWithArch(runtime.GOARCH)
}

// newTestConfigWithArch returns bundle configuration for a selected architecture.
func newTestConfigWithArch(arch string) *UDSBundleConfig {
	opts := ConfigOptions{Architecture: arch, TmpDir: os.TempDir(), Concurrency: 10}
	return &UDSBundleConfig{Global: &GlobalOptions{}, Options: &opts}
}

// pushTo returns hooks that direct pushes to an in-memory test target.
func pushTo(target oras.Target) PushHooks {
	return PushHooks{
		ToOrasTarget: func(context.Context, string, *PushOptions) (oras.Target, error) {
			return target, nil
		},
	}
}

// bundleDefinitionContainsLayerTitle reports whether a bundle definition includes a layer title.
func bundleDefinitionContainsLayerTitle(t *testing.T, entries map[string][]byte, title string) bool {
	t.Helper()
	var idx udsoci.OciIndex
	require.NoError(t, json.Unmarshal(entries["oci/index.json"], &idx))
	entry, _, err := udsoci.FindBundleDefinitionEntry(idx)
	require.NoError(t, err)
	manifestBytes := entries["oci/blobs/sha256/"+strings.TrimPrefix(entry.Digest, "sha256:")]
	var manifest udsoci.OciImageManifest
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
	for _, layer := range manifest.Layers {
		if layer.Annotations["org.opencontainers.image.title"] == title {
			_, ok := entries["oci/blobs/sha256/"+strings.TrimPrefix(layer.Digest, "sha256:")]
			return ok
		}
	}
	return false
}

// readTarZstEntries returns the files contained in a test artifact.
func readTarZstEntries(t *testing.T, path string) map[string][]byte {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	archive := archives.CompressedArchive{Extraction: archives.Tar{}, Compression: archives.Zstd{}}
	entries := map[string][]byte{}
	err = archive.Extract(t.Context(), f, func(_ context.Context, info archives.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		r, err := info.Open()
		if err != nil {
			return err
		}
		defer func() { _ = r.Close() }()
		entries[info.NameInArchive], err = io.ReadAll(r)
		return err
	})
	require.NoError(t, err)
	return entries
}

// buildBundleArtifact assembles a bundle artifact from test HCL and package sources.
func buildBundleArtifact(t *testing.T, bundleHCL string, valuesFiles map[string][]string, pkgSources []string) string {
	t.Helper()
	root := t.TempDir()
	ociDir := filepath.Join(root, "oci")
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))
	require.NoError(t, udsoci.WriteOCILayout(filepath.Join(ociDir, "oci-layout")))
	writeBlob := func(data []byte) string {
		sum := sha256.Sum256(data)
		h := hex.EncodeToString(sum[:])
		require.NoError(t, os.WriteFile(filepath.Join(blobDir, h), data, tmpFilePerm))
		return "sha256:" + h
	}

	layers := []udsoci.OciDescriptor{{
		MediaType:   MediaTypeBundleHCL,
		Digest:      writeBlob([]byte(bundleHCL)),
		Size:        int64(len(bundleHCL)),
		Annotations: map[string]string{"org.opencontainers.image.title": BundleFileName},
	}}
	for packageName, files := range valuesFiles {
		for i, content := range files {
			layers = append(layers, udsoci.OciDescriptor{
				MediaType:   MediaTypeBundleValuesYAML,
				Digest:      writeBlob([]byte(content)),
				Size:        int64(len(content)),
				Annotations: map[string]string{"org.opencontainers.image.title": fmt.Sprintf("values/%s/%d.yaml", packageName, i)},
			})
		}
	}
	emptyConfig := []byte("{}")
	emptyConfigDigest := writeBlob(emptyConfig)
	definition := udsoci.OciImageManifest{
		SchemaVersion: 2,
		Config:        udsoci.OciDescriptor{MediaType: "application/vnd.oci.empty.v1+json", Digest: emptyConfigDigest, Size: int64(len(emptyConfig))},
		Layers:        layers,
	}
	definitionBytes, err := json.Marshal(definition)
	require.NoError(t, err)
	manifests := []udsoci.OciManifest{{
		MediaType:    "application/vnd.oci.image.manifest.v1+json",
		ArtifactType: MediaTypeBundleDefinition,
		Digest:       writeBlob(definitionBytes),
		Size:         int64(len(definitionBytes)),
	}}
	for _, source := range pkgSources {
		packageData := []byte("fake package: " + source)
		packageManifest := udsoci.OciImageManifest{
			SchemaVersion: 2,
			Config:        udsoci.OciDescriptor{MediaType: "application/vnd.oci.empty.v1+json", Digest: emptyConfigDigest, Size: int64(len(emptyConfig))},
			Layers: []udsoci.OciDescriptor{{
				MediaType:   udsoci.MediaTypeZarfLayer,
				Digest:      writeBlob(packageData),
				Size:        int64(len(packageData)),
				Annotations: map[string]string{"org.opencontainers.image.title": "zarf.yaml"},
			}},
		}
		packageManifestBytes, err := json.Marshal(packageManifest)
		require.NoError(t, err)
		refName := source
		if IsOCIReference(source) {
			refName = TrimScheme(source)
		}
		manifests = append(manifests, udsoci.OciManifest{
			MediaType:   "application/vnd.oci.image.manifest.v1+json",
			Digest:      writeBlob(packageManifestBytes),
			Size:        int64(len(packageManifestBytes)),
			Annotations: map[string]string{"org.opencontainers.image.ref.name": refName},
		})
	}
	require.NoError(t, udsoci.WriteOCIIndex(filepath.Join(ociDir, "index.json"), udsoci.NewBundleIndex(manifests, "amd64")))
	outPath := filepath.Join(t.TempDir(), "bundle.tar.zst")
	require.NoError(t, artifact.WriteTarZst(t.Context(), iostreams.IOStreams{}, outPath, root))
	return outPath
}
