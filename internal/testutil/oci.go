// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/filesystem"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	godigest "github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// writeTestBlob writes b to blobDir/<sha256hex> and returns the hex digest.
func writeTestBlob(t *testing.T, blobDir string, b []byte) string {
	t.Helper()
	sum := sha256.Sum256(b)
	h := hex.EncodeToString(sum[:])
	require.NoError(t, os.WriteFile(filepath.Join(blobDir, h), b, filesystem.PrivateFileMode))
	return h
}

// ZarfLayoutDigests holds per-component SHA-256 digests written by WriteZarfLikeOCILayout.
type ZarfLayoutDigests struct {
	// ComponentHexes maps component names to their layer blob's SHA-256 hex digest.
	ComponentHexes map[string]string
}

// zarfComponent models component metadata in a test Zarf package.
type zarfComponent struct {
	Name     string `yaml:"name"`
	Required *bool  `yaml:"required,omitempty"`
}

// zarfPackage models the package metadata used by OCI test fixtures.
type zarfPackage struct {
	Components []zarfComponent `yaml:"components"`
}

// WriteZarfLikeOCILayout creates a complete OCI image layout directory in layoutDir
// that mimics a single-manifest Zarf package. required and optional specify component
// names. Returns zarfLayoutDigests with a map of component name → sha256 hex digest.
func WriteZarfLikeOCILayout(t *testing.T, layoutDir string, required, optional []string) ZarfLayoutDigests {
	t.Helper()

	blobDir := filepath.Join(layoutDir, "blobs", "sha256")
	_, err := udsoci.CreateStore(layoutDir)
	require.NoError(t, err)

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

	layers := []ocispec.Descriptor{{
		MediaType:   "application/vnd.zarf.layer.v1.blob",
		Digest:      godigest.NewDigestFromEncoded(godigest.SHA256, zarfHash),
		Size:        int64(len(zarfBytes)),
		Annotations: map[string]string{"org.opencontainers.image.title": "zarf.yaml"},
	}}

	compDigests := map[string]string{}
	for _, name := range append(required, optional...) {
		data := []byte(name + " component data")
		h := writeTestBlob(t, blobDir, data)
		compDigests[name] = h
		layers = append(layers, ocispec.Descriptor{
			MediaType:   "application/vnd.zarf.layer.v1.blob",
			Digest:      godigest.NewDigestFromEncoded(godigest.SHA256, h),
			Size:        int64(len(data)),
			Annotations: map[string]string{"org.opencontainers.image.title": "components/" + name + ".tar"},
		})
	}

	im := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Config: ocispec.Descriptor{
			MediaType: "application/vnd.zarf.config.v1+json",
			Digest:    godigest.NewDigestFromEncoded(godigest.SHA256, configHash),
			Size:      int64(len(configBytes)),
		},
		Layers: layers,
	}
	manifestBytes, err := json.Marshal(im)
	require.NoError(t, err)
	manifestHash := writeTestBlob(t, blobDir, manifestBytes)

	idx := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: "application/vnd.oci.image.index.v1+json",
		Manifests: []ocispec.Descriptor{{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    godigest.NewDigestFromEncoded(godigest.SHA256, manifestHash),
			Size:      int64(len(manifestBytes)),
		}},
	}
	require.NoError(t, udsoci.WriteIndex(filepath.Join(layoutDir, "index.json"), &idx))

	return ZarfLayoutDigests{ComponentHexes: compDigests}
}
