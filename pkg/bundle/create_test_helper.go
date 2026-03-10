// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// ociLayoutDigests holds the hex-encoded sha256 digests written by writeMinimalOCILayout.
type ociLayoutDigests struct {
	ManifestHex string
	ConfigHex   string
	LayerHex    string
}

// zarfLayoutDigests holds per-component sha256 hex digests written by WriteZarfLikeOCILayout.
type zarfLayoutDigests struct {
	// ComponentHexes maps component name → sha256 hex of its layer blob.
	ComponentHexes map[string]string
}

// writeMultiArchOCILayout creates a multi-platform OCI image index under layoutDir
// with one random image per arch in arches (OS set to "linux").
// Use this to test architecture-filtering behaviour in bundle create.
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
				Platform: &v1.Platform{
					OS:           "linux",
					Architecture: arch,
				},
			},
		}
	}
	idx := mutate.AppendManifests(empty.Index, addendums...)

	_, err := layout.Write(layoutDir, idx)
	require.NoError(t, err)
}

// writeMinimalOCILayout creates a minimal but valid OCI image layout under layoutDir
// using the go-containerregistry layout package. It returns the sha256 hex digests
// of the manifest, config, and layer blobs it wrote.
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

	return ociLayoutDigests{
		ManifestHex: manifestHash.Hex,
		ConfigHex:   configHash.Hex,
		LayerHex:    layerHash.Hex,
	}
}

// OCI JSON types used only by WriteZarfLikeOCILayout to avoid depending on
// unexported bundle types.

type ociDescriptorTest struct {
	MediaType   string            `json:"mediaType,omitempty"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type ociImageManifestTest struct {
	SchemaVersion int                 `json:"schemaVersion"`
	MediaType     string              `json:"mediaType,omitempty"`
	Config        ociDescriptorTest   `json:"config"`
	Layers        []ociDescriptorTest `json:"layers"`
}

type ociIndexTest struct {
	SchemaVersion int                 `json:"schemaVersion"`
	MediaType     string              `json:"mediaType"`
	Manifests     []ociDescriptorTest `json:"manifests"`
}

type ociLayoutTest struct {
	ImageLayoutVersion string `json:"imageLayoutVersion"`
}

type zarfComponent struct {
	Name     string `yaml:"name"`
	Required *bool  `yaml:"required,omitempty"`
}

type zarfPackage struct {
	Components []zarfComponent `yaml:"components"`
}

// writeTestBlob writes b to blobDir/<sha256hex> and returns the hex digest.
func writeTestBlob(t *testing.T, blobDir string, b []byte) string {
	t.Helper()
	sum := sha256.Sum256(b)
	h := hex.EncodeToString(sum[:])
	require.NoError(t, os.WriteFile(filepath.Join(blobDir, h), b, 0o644))
	return h
}

// WriteZarfLikeOCILayout creates a complete OCI image layout directory in layoutDir
// that mimics a single-manifest Zarf package. required and optional specify component
// names. Returns zarfLayoutDigests with a map of component name → sha256 hex digest.
func WriteZarfLikeOCILayout(t *testing.T, layoutDir string, required, optional []string) zarfLayoutDigests {
	t.Helper()

	blobDir := filepath.Join(layoutDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, 0o755))

	reqTrue := true
	reqFalse := false
	components := make([]zarfComponent, 0, len(required)+len(optional))
	for _, name := range required {
		components = append(components, zarfComponent{Name: name, Required: &reqTrue})
	}
	for _, name := range optional {
		components = append(components, zarfComponent{Name: name, Required: &reqFalse})
	}

	zarfBytes, err := yaml.Marshal(zarfPackage{Components: components})
	require.NoError(t, err)
	zarfHash := writeTestBlob(t, blobDir, zarfBytes)

	configBytes := []byte("{}")
	configHash := writeTestBlob(t, blobDir, configBytes)

	layers := []ociDescriptorTest{{
		MediaType:   "application/vnd.zarf.layer.v1.blob",
		Digest:      "sha256:" + zarfHash,
		Size:        int64(len(zarfBytes)),
		Annotations: map[string]string{"org.opencontainers.image.title": "zarf.yaml"},
	}}

	compDigests := map[string]string{}
	for _, name := range append(required, optional...) {
		data := []byte(name + " component data")
		h := writeTestBlob(t, blobDir, data)
		compDigests[name] = h
		layers = append(layers, ociDescriptorTest{
			MediaType:   "application/vnd.zarf.layer.v1.blob",
			Digest:      "sha256:" + h,
			Size:        int64(len(data)),
			Annotations: map[string]string{"org.opencontainers.image.title": "components/" + name + ".tar"},
		})
	}

	im := ociImageManifestTest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config: ociDescriptorTest{
			MediaType: "application/vnd.zarf.config.v1+json",
			Digest:    "sha256:" + configHash,
			Size:      int64(len(configBytes)),
		},
		Layers: layers,
	}
	manifestBytes, err := json.Marshal(im)
	require.NoError(t, err)
	manifestHash := writeTestBlob(t, blobDir, manifestBytes)

	idx := ociIndexTest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.index.v1+json",
		Manifests: []ociDescriptorTest{{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    "sha256:" + manifestHash,
			Size:      int64(len(manifestBytes)),
		}},
	}
	idxBytes, err := json.Marshal(idx)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(layoutDir, "index.json"), idxBytes, 0o644))

	ociLayoutBytes, err := json.Marshal(ociLayoutTest{ImageLayoutVersion: "1.0.0"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(layoutDir, "oci-layout"), ociLayoutBytes, 0o644))

	return zarfLayoutDigests{ComponentHexes: compDigests}
}

// bundleDefinitionContainsLayerTitle reports whether the bundle definition manifest
// (identified by artifactType == MediaTypeBundleDefinition) contains a layer with
// the given org.opencontainers.image.title AND the corresponding blob is present.
func bundleDefinitionContainsLayerTitle(t *testing.T, entries map[string][]byte, title string) bool {
	t.Helper()

	idxBytes, ok := entries["oci/index.json"]
	require.True(t, ok, "oci/index.json not found in bundle")

	var idx struct {
		Manifests []struct {
			Digest       string `json:"digest"`
			ArtifactType string `json:"artifactType"`
		} `json:"manifests"`
	}
	require.NoError(t, json.Unmarshal(idxBytes, &idx))

	for _, m := range idx.Manifests {
		if m.ArtifactType != MediaTypeBundleDefinition {
			continue
		}
		hex := strings.TrimPrefix(m.Digest, "sha256:")
		manifestBytes, ok := entries["oci/blobs/sha256/"+hex]
		if !ok {
			continue
		}
		var im struct {
			Layers []struct {
				Digest      string            `json:"digest"`
				Annotations map[string]string `json:"annotations"`
			} `json:"layers"`
		}
		require.NoError(t, json.Unmarshal(manifestBytes, &im))
		for _, l := range im.Layers {
			if l.Annotations["org.opencontainers.image.title"] == title {
				layerHex := strings.TrimPrefix(l.Digest, "sha256:")
				_, ok := entries["oci/blobs/sha256/"+layerHex]
				return ok
			}
		}
	}
	return false
}
