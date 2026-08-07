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
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/mholt/archives"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	oras "oras.land/oras-go/v2"
)

const (
	tempDirPerm fs.FileMode = 0o700
	tmpFilePerm fs.FileMode = 0o600
)

// ociLayoutDigests records blob digests in a generated test layout.
type ociLayoutDigests struct {
	ManifestHex string
	ConfigHex   string
	LayerHex    string
}

// zarfLayoutDigests records component blob digests in a Zarf test layout.
type zarfLayoutDigests struct {
	ComponentHexes map[string]string
}

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

// writeMinimalOCILayout creates a valid minimal OCI layout for bundle tests.
func writeMinimalOCILayout(t *testing.T, layoutDir string) ociLayoutDigests {
	t.Helper()
	img, err := random.Image(64, 1)
	require.NoError(t, err)
	lp, err := layout.Write(layoutDir, empty.Index)
	require.NoError(t, err)
	require.NoError(t, lp.AppendImage(img))
	manifestHash, err := img.Digest()
	require.NoError(t, err)
	configHash, err := img.ConfigName()
	require.NoError(t, err)
	layers, err := img.Layers()
	require.NoError(t, err)
	require.Len(t, layers, 1)
	layerHash, err := layers[0].Digest()
	require.NoError(t, err)
	return ociLayoutDigests{ManifestHex: manifestHash.Hex, ConfigHex: configHash.Hex, LayerHex: layerHash.Hex}
}

// writeMultiArchOCILayout creates an OCI layout with manifests for each architecture.
func writeMultiArchOCILayout(t *testing.T, layoutDir string, arches []string) {
	t.Helper()
	addendums := make([]mutate.IndexAddendum, len(arches))
	for i, arch := range arches {
		img, err := random.Image(64, 1)
		require.NoError(t, err)
		addendums[i] = mutate.IndexAddendum{
			Add: img,
			Descriptor: v1.Descriptor{
				MediaType: types.OCIManifestSchema1,
				Platform:  &v1.Platform{OS: "linux", Architecture: arch},
			},
		}
	}
	_, err := layout.Write(layoutDir, mutate.AppendManifests(empty.Index, addendums...))
	require.NoError(t, err)
}

// zarfComponent models component metadata in a test Zarf package.
type zarfComponent struct {
	Name     string `yaml:"name"`
	Required *bool  `yaml:"required,omitempty"`
}

// WriteZarfLikeOCILayout creates a Zarf-compatible OCI layout for bundle tests.
func WriteZarfLikeOCILayout(t *testing.T, layoutDir string, required, optional []string) zarfLayoutDigests {
	t.Helper()
	blobDir := filepath.Join(layoutDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))
	writeBlob := func(data []byte) string {
		sum := sha256.Sum256(data)
		h := hex.EncodeToString(sum[:])
		require.NoError(t, os.WriteFile(filepath.Join(blobDir, h), data, tmpFilePerm))
		return h
	}

	reqTrue, reqFalse := true, false
	components := make([]zarfComponent, 0, len(required)+len(optional))
	for _, name := range required {
		components = append(components, zarfComponent{Name: name, Required: &reqTrue})
	}
	for _, name := range optional {
		components = append(components, zarfComponent{Name: name, Required: &reqFalse})
	}
	zarfBytes, err := yaml.Marshal(struct {
		Components []zarfComponent `yaml:"components"`
	}{Components: components})
	require.NoError(t, err)
	zarfHash := writeBlob(zarfBytes)
	configBytes := []byte("{}")
	configHash := writeBlob(configBytes)
	layers := []udsoci.OciDescriptor{{
		MediaType:   udsoci.MediaTypeZarfLayer,
		Digest:      "sha256:" + zarfHash,
		Size:        int64(len(zarfBytes)),
		Annotations: map[string]string{"org.opencontainers.image.title": "zarf.yaml"},
	}}
	componentHexes := map[string]string{}
	for _, name := range append(required, optional...) {
		data := []byte(name + " component data")
		h := writeBlob(data)
		componentHexes[name] = h
		layers = append(layers, udsoci.OciDescriptor{
			MediaType:   udsoci.MediaTypeZarfLayer,
			Digest:      "sha256:" + h,
			Size:        int64(len(data)),
			Annotations: map[string]string{"org.opencontainers.image.title": "components/" + name + ".tar"},
		})
	}
	manifest := udsoci.OciImageManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config: udsoci.OciDescriptor{
			MediaType: "application/vnd.zarf.config.v1+json",
			Digest:    "sha256:" + configHash,
			Size:      int64(len(configBytes)),
		},
		Layers: layers,
	}
	manifestBytes, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestHash := writeBlob(manifestBytes)
	idx := &udsoci.OciIndex{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.index.v1+json",
		Manifests: []udsoci.OciManifest{{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    "sha256:" + manifestHash,
			Size:      int64(len(manifestBytes)),
		}},
	}
	require.NoError(t, udsoci.WriteOCIIndex(filepath.Join(layoutDir, "index.json"), idx))
	require.NoError(t, udsoci.WriteOCILayout(filepath.Join(layoutDir, "oci-layout")))
	return zarfLayoutDigests{ComponentHexes: componentHexes}
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
