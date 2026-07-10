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
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	oraci "oras.land/oras-go/v2/content/oci"
)

// pullFrom returns PullHooks that inject src as the pull source via the
// ToOrasTarget seam, the same path production uses to resolve a registry.
func pullFrom(src oras.Target) PullHooks {
	return PullHooks{
		ToOrasTarget: func(context.Context, string, *PullOptions) (oras.Target, error) { return src, nil },
	}
}

func TestBundleNameFromIndex_HappyPath(t *testing.T) {
	t.Parallel()
	ociDir := writeBundleOCILayout(t, "my-bundle", "0.2.0")

	idxBytes, err := os.ReadFile(filepath.Join(ociDir, "index.json"))
	require.NoError(t, err)
	var idx ociIndex
	require.NoError(t, json.Unmarshal(idxBytes, &idx))

	name, err := bundleNameFromDefinitionLayer(t.Context(), iostreams.IOStreams{}, ociDir, idx, "amd64")
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
	name, err := bundleNameFromDefinitionLayer(t.Context(), iostreams.IOStreams{}, ociDir, idx, "")
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

	_, err := bundleNameFromDefinitionLayer(t.Context(), iostreams.IOStreams{}, ociDir, idx, "amd64")
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

	_, err = bundleNameFromDefinitionLayer(t.Context(), iostreams.IOStreams{}, ociDir, idx, "amd64")
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
				Config:    cfg,
				PullHooks: pullFrom(srcStore),
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
		Config:    cfg,
		PullHooks: pullFrom(srcStore),
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

	tarball, err := Create(t.Context(), CreateOptions{
		Config:     newTestConfig(),
		BundleFile: bundleFile,
		Streams:    iostreams.New(nil, nil, os.Stderr),
	})
	require.NoError(t, err)

	// Push to an in-memory store.
	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)
	pushCfg := newTestConfig()
	pushCfg.Options.TmpDir = t.TempDir()
	_, pushErr := Push(t.Context(), tarball.OutputPath, "example.com/test/pull-test:1.0.0", PushOptions{
		Config:    pushCfg,
		PushHooks: pushTo(store),
	})
	require.NoError(t, pushErr)

	// Pull from the same store.
	outDir := t.TempDir()
	pullCfg := newTestConfig()
	pullCfg.Options.TmpDir = t.TempDir()
	result, err := Pull(t.Context(), "example.com/test/pull-test:1.0.0", outDir, PullOptions{
		Config:    pullCfg,
		PullHooks: pullFrom(store),
	})
	require.NoError(t, err)

	expectedName := fmt.Sprintf("uds-bundle-pull-test-%s-1.0.0.tar.zst", runtime.GOARCH)
	assert.Equal(t, filepath.Join(outDir, expectedName), result.OutputPath)
	assert.Equal(t, "example.com/test/pull-test:1.0.0", result.OCIReference)
	_, statErr := os.Stat(result.OutputPath)
	require.NoError(t, statErr, "pulled tarball should exist on disk")
}

func TestPullPackage_RoundTrip(t *testing.T) {
	t.Parallel()

	// Build a minimal OCI layout in a source store and tag it.
	srcDir := t.TempDir()
	writeMinimalOCILayout(t, srcDir)

	srcStore, err := oraci.New(srcDir)
	require.NoError(t, err)

	// Read the root descriptor from index.json and tag it so oras.Copy can resolve it.
	root, err := packageRootDescriptor(srcDir)
	require.NoError(t, err)
	require.NoError(t, srcStore.Tag(t.Context(), root, "1.0.0"))

	targetDir := t.TempDir()
	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := NewDefaultPuller().PullPackage(t.Context(), "example.com/test/pkg:1.0.0", targetDir, PullOptions{
		Config:    cfg,
		PullHooks: pullFrom(srcStore),
	})
	require.NoError(t, err)
	assert.Equal(t, "example.com/test/pkg:1.0.0", result.OCIReference)
	assert.Equal(t, filepath.Join(targetDir, "oci"), result.OutputPath)

	_, statErr := os.Stat(filepath.Join(result.OutputPath, "index.json"))
	require.NoError(t, statErr, "index.json should exist in pulled OCI dir")
}

func TestPullPackage_EmptyOCIReference(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := NewDefaultPuller().PullPackage(t.Context(), "", t.TempDir(), PullOptions{
		Config: cfg,
	})
	require.ErrorContains(t, err, "ociReference must not be empty")
	assert.Nil(t, result)
}

func TestPullPackage_EmptyTargetDir(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := NewDefaultPuller().PullPackage(t.Context(), "example.com/test/pkg:1.0.0", "", PullOptions{
		Config: cfg,
	})
	require.ErrorContains(t, err, "targetDir must not be empty")
	assert.Nil(t, result)
}

func TestPullHooks_ModifyOrasSettings(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	writeMinimalOCILayout(t, srcDir)

	srcStore, err := oraci.New(srcDir)
	require.NoError(t, err)

	root, err := packageRootDescriptor(srcDir)
	require.NoError(t, err)
	require.NoError(t, srcStore.Tag(t.Context(), root, "1.0.0"))

	hookCalled := false
	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := NewDefaultPuller().PullPackage(t.Context(), "example.com/test/pkg:1.0.0", t.TempDir(), PullOptions{
		Config: cfg,
		PullHooks: PullHooks{
			ToOrasTarget: func(context.Context, string, *PullOptions) (oras.Target, error) { return srcStore, nil },
			ModifyOrasSettings: func(_ context.Context, co *oras.CopyOptions) error {
				hookCalled = true
				co.Concurrency = 3
				return nil
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, hookCalled, "ModifyOrasSettings hook should have been called")
}

func TestPull_SelectsRequestedArchitectureFromRootIndex(t *testing.T) {
	t.Parallel()

	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)
	ref := "example.com/test/arch-select:1.0.0"
	pushArchTestBundle(t, store, ref, createArchTestBundle(t, "arch-select", "1.0.0", "amd64"))
	pushArchTestBundle(t, store, ref, createArchTestBundle(t, "arch-select", "1.0.0", "arm64"))

	for _, arch := range []string{"amd64", "arm64"} {
		outDir := t.TempDir()
		cfg := newTestConfigWithArch(arch)
		cfg.Options.TmpDir = t.TempDir()
		result, err := Pull(t.Context(), ref, outDir, PullOptions{
			Config:    cfg,
			PullHooks: pullFrom(store),
		})
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(outDir, fmt.Sprintf("uds-bundle-arch-select-%s-1.0.0.tar.zst", arch)), result.OutputPath)

		entries := readTarZstEntries(t, result.OutputPath)
		var idx ociIndex
		require.NoError(t, json.Unmarshal(entries["oci/index.json"], &idx))
		assert.Equal(t, arch, idx.Annotations[AnnotationBundleArchitecture])
	}
}

func TestPull_ErrorsWhenArchitectureMissing(t *testing.T) {
	t.Parallel()

	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)
	ref := "example.com/test/one-arch:1.0.0"
	pushArchTestBundle(t, store, ref, createArchTestBundle(t, "one-arch", "1.0.0", "amd64"))

	cfg := newTestConfigWithArch("arm64")
	cfg.Options.TmpDir = t.TempDir()
	_, err = Pull(t.Context(), ref, t.TempDir(), PullOptions{
		Config:    cfg,
		PullHooks: pullFrom(store),
	})
	require.ErrorContains(t, err, `no bundle for architecture "arm64"`)
	require.ErrorContains(t, err, "amd64")
}

func TestPull_RoundTripsChildIndexBytes(t *testing.T) {
	t.Parallel()

	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)
	ref := "example.com/test/round-trip:1.0.0"
	tarball := createArchTestBundle(t, "round-trip", "1.0.0", runtime.GOARCH)
	pushArchTestBundle(t, store, ref, tarball)

	outDir := t.TempDir()
	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := Pull(t.Context(), ref, outDir, PullOptions{
		Config:    cfg,
		PullHooks: pullFrom(store),
	})
	require.NoError(t, err)

	created := readTarZstEntries(t, tarball)
	pulled := readTarZstEntries(t, result.OutputPath)
	assert.Equal(t, created["oci/index.json"], pulled["oci/index.json"],
		"pulled bundle index must round-trip byte-identically")
}
