// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/content"
	oraci "oras.land/oras-go/v2/content/oci"
)

func TestBundleNameFromIndex_HappyPath(t *testing.T) {
	t.Parallel()
	ociDir := writeBundleOCILayout(t, "my-bundle", "0.2.0")

	idxBytes, err := os.ReadFile(filepath.Join(ociDir, "index.json"))
	require.NoError(t, err)
	var idx ociIndex
	require.NoError(t, json.Unmarshal(idxBytes, &idx))

	name, err := bundleNameFromDefinitionLayer(context.Background(), iostreams.IOStreams{}, ociDir, idx, "amd64")
	require.NoError(t, err)
	assert.Equal(t, "uds-bundle-my-bundle-amd64-0.2.0.tar.zst", name)
}

func TestBundleNameFromIndex_ArchFallback(t *testing.T) {
	t.Parallel()
	ociDir := writeBundleOCILayout(t, "my-bundle", "0.1.0")

	idxBytes, err := os.ReadFile(filepath.Join(ociDir, "index.json"))
	require.NoError(t, err)
	var idx ociIndex
	require.NoError(t, json.Unmarshal(idxBytes, &idx))

	// Empty arch should fall back to runtime.GOARCH.
	name, err := bundleNameFromDefinitionLayer(context.Background(), iostreams.IOStreams{}, ociDir, idx, "")
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("uds-bundle-my-bundle-%s-0.1.0.tar.zst", runtime.GOARCH), name)
}

func TestBundleNameFromIndex_NoBundleDefinitionManifest(t *testing.T) {
	t.Parallel()
	ociDir := t.TempDir()
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

	// Index with a manifest that has no ArtifactType set.
	manifestBytes := []byte(`{"schemaVersion":2,"config":{"digest":"sha256:abc","size":2},"layers":[]}`)
	manifestHex := writeTestBlob(t, blobDir, manifestBytes)

	idx := ociIndex{
		SchemaVersion: 2,
		Manifests: []ociManifest{{
			Digest: "sha256:" + manifestHex,
			Size:   int64(len(manifestBytes)),
		}},
	}

	_, err := bundleNameFromDefinitionLayer(context.Background(), iostreams.IOStreams{}, ociDir, idx, "amd64")
	require.ErrorContains(t, err, "bundle definition manifest not found")
}

func TestBundleNameFromIndex_NoHCLLayer(t *testing.T) {
	t.Parallel()
	ociDir := t.TempDir()
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

	// Config manifest with no HCL layer.
	cfgManifest := ociImageManifest{
		SchemaVersion: 2,
		Config:        ociDescriptor{Digest: "sha256:abc", Size: 2},
		Layers:        []ociDescriptor{},
	}
	cfgBytes, err := json.Marshal(cfgManifest)
	require.NoError(t, err)
	cfgHex := writeTestBlob(t, blobDir, cfgBytes)

	idx := ociIndex{
		SchemaVersion: 2,
		Manifests: []ociManifest{{
			ArtifactType: MediaTypeBundleDefinition,
			Digest:       "sha256:" + cfgHex,
			Size:         int64(len(cfgBytes)),
		}},
	}

	_, err = bundleNameFromDefinitionLayer(context.Background(), iostreams.IOStreams{}, ociDir, idx, "amd64")
	require.ErrorContains(t, err, "bundle HCL layer not found")
}

func TestPull_NonUDSBundle(t *testing.T) {
	t.Parallel()

	// Build an in-memory OCI store with a plain image manifest (no bundle definition).
	// Config and manifest must use real sha256 digests so oras.Copy can copy them.
	ociDir := t.TempDir()
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

	configBytes := []byte("{}")
	configHex := writeTestBlob(t, blobDir, configBytes)

	manifest := ociImageManifest{
		SchemaVersion: 2,
		Config: ociDescriptor{
			MediaType: "application/vnd.oci.empty.v1+json",
			Digest:    "sha256:" + configHex,
			Size:      int64(len(configBytes)),
		},
		Layers: []ociDescriptor{},
	}
	manifestBytes, err := json.Marshal(manifest)
	require.NoError(t, err)
	writeTestBlob(t, blobDir, manifestBytes)

	srcStore, err := oraci.New(ociDir)
	require.NoError(t, err)
	manifestDesc := content.NewDescriptorFromBytes("application/vnd.oci.image.manifest.v1+json", manifestBytes)
	require.NoError(t, srcStore.Tag(t.Context(), manifestDesc, "v1.0.0"))

	tests := []struct {
		name        string
		outputDir   string
		expectedErr string
	}{
		{"with output dir", t.TempDir(), "does not appear to be a UDS bundle"},
		{"without output dir", "", "targetDir must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := newTestConfig()
			cfg.Options.TmpDir = t.TempDir()
			result, err := Pull(t.Context(), "example.com/test/not-a-bundle:v1.0.0", tt.outputDir, PullOptions{
				Config:     cfg,
				remoteRepo: srcStore,
			})
			require.ErrorContains(t, err, tt.expectedErr)
			assert.Nil(t, result)
		})
	}
}

func TestPull_TagNotFound(t *testing.T) {
	t.Parallel()

	srcStore, err := oraci.New(t.TempDir())
	require.NoError(t, err)

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	ref := "example.com/test/bundle:v1.0.0"
	result, err := Pull(t.Context(), ref, t.TempDir(), PullOptions{
		Config:     cfg,
		remoteRepo: srcStore,
	})

	require.ErrorContains(t, err, ref)
	assert.Nil(t, result)
}

func TestPull_HappyPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeMinimalOCILayout(t, filepath.Join(dir, "localpkg"))
	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "pull-test"
  version = "1.0.0"
}
package "pkg1" {
  source = "localpkg"
}
`), tmpFilePerm))

	tarball, err := Create(context.Background(), CreateOptions{
		Config:     newTestConfig(),
		BundleFile: bundleFile,
		Streams:    iostreams.IOStreams{ErrOut: os.Stderr},
	})
	require.NoError(t, err)

	// Push to an in-memory store.
	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)
	pushCfg := newTestConfig()
	pushCfg.Options.TmpDir = t.TempDir()
	_, pushErr := Push(context.Background(), tarball.OutputPath, "example.com/test/pull-test:1.0.0", PushOptions{
		Config:     pushCfg,
		remoteRepo: store,
	})
	require.NoError(t, pushErr)

	// Pull from the same store.
	outDir := t.TempDir()
	pullCfg := newTestConfig()
	pullCfg.Options.TmpDir = t.TempDir()
	result, err := Pull(t.Context(), "example.com/test/pull-test:1.0.0", outDir, PullOptions{
		Config:     pullCfg,
		remoteRepo: store,
	})
	require.NoError(t, err)

	expectedName := fmt.Sprintf("uds-bundle-pull-test-%s-1.0.0.tar.zst", runtime.GOARCH)
	assert.Equal(t, filepath.Join(outDir, expectedName), result.OutputPath)
	assert.Equal(t, "example.com/test/pull-test:1.0.0", result.OCIReference)
	_, statErr := os.Stat(result.OutputPath)
	require.NoError(t, statErr, "pulled tarball should exist on disk")
}
