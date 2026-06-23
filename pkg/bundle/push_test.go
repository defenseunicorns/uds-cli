// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	oras "oras.land/oras-go/v2"
	oraci "oras.land/oras-go/v2/content/oci"
)

// pushTo returns PushHooks that inject target as the push destination via the
// ToOrasTarget seam, the same path production uses to resolve a registry.
func pushTo(target oras.Target) PushHooks {
	return PushHooks{
		ToOrasTarget: func(context.Context, string, *PushOptions) (oras.Target, error) { return target, nil },
	}
}

func TestPush_NoOCILayout(t *testing.T) {
	t.Parallel()

	// Build a tar.zst with no oci/ directory at all, simulating a v0 bundle.
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "some-file.txt"), []byte("not a bundle"), tmpFilePerm))

	tarball := filepath.Join(t.TempDir(), "v0-bundle.tar.zst")
	require.NoError(t, writeTarZst(t.Context(), iostreams.IOStreams{}, tarball, srcDir))

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := Push(t.Context(), tarball, "example.com/test/v0-bundle:v1.0.0", PushOptions{
		Config: cfg,
	})

	require.ErrorContains(t, err, "does not appear to be a UDS bundle")
	require.ErrorContains(t, err, "no OCI layout found")
	assert.Nil(t, result)
}

func TestPush_NonUDSBundle(t *testing.T) {
	t.Parallel()

	// Build a tar.zst containing a valid OCI layout but with no bundle definition manifest.
	srcDir := t.TempDir()
	ociDir := filepath.Join(srcDir, "oci")
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

	// Write a plain image manifest with no ArtifactType.
	manifestBytes := []byte(`{"schemaVersion":2,"config":{"digest":"sha256:abc","size":2},"layers":[]}`)
	manifestHex := writeTestBlob(t, blobDir, manifestBytes)
	idxBytes, err := json.Marshal(ociIndex{
		SchemaVersion: 2,
		Manifests: []ociManifest{{
			Digest: "sha256:" + manifestHex,
			Size:   int64(len(manifestBytes)),
		}},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(ociDir, "index.json"), idxBytes, tmpFilePerm))

	tarball := filepath.Join(t.TempDir(), "not-a-bundle.tar.zst")
	require.NoError(t, writeTarZst(t.Context(), iostreams.IOStreams{}, tarball, srcDir))

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := Push(t.Context(), tarball, "example.com/test/not-a-bundle:v1.0.0", PushOptions{
		Config: cfg,
	})

	require.Error(t, err)
	require.ErrorContains(t, err, "does not appear to be a UDS bundle")
	assert.Nil(t, result)
}

func TestPush_HappyPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeMinimalOCILayout(t, filepath.Join(dir, "localpkg"))
	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "push-test"
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

	dst, err := oraci.New(t.TempDir())
	require.NoError(t, err)

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := Push(t.Context(), tarball.OutputPath, "example.com/test/push-test:1.0.0", PushOptions{
		Config:    cfg,
		PushHooks: pushTo(dst),
	})
	require.NoError(t, err)
	assert.Equal(t, "example.com/test/push-test:1.0.0", result.OCIReference)

	// Verify the manifest is accessible at the expected tag in the in-memory store.
	_, err = dst.Resolve(t.Context(), "1.0.0")
	require.NoError(t, err, "manifest should be present in store after push")
}

func TestPush_TarballNotFound(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := Push(t.Context(), "/nonexistent/bundle.tar.zst", "example.com/test/bundle:v1.0.0", PushOptions{
		Config: cfg,
	})

	require.Error(t, err)
	require.ErrorContains(t, err, "no such file or directory")
	assert.Nil(t, result)
}

func TestPush_InvalidOCIReference(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a minimal bundle tarball.
	writeMinimalOCILayout(t, filepath.Join(dir, "localpkg"))
	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "push-test"
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

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := Push(t.Context(), tarball.OutputPath, ":::invalid:::", PushOptions{
		Config: cfg,
	})

	require.Error(t, err)
	require.ErrorContains(t, err, "invalid reference")
	assert.Nil(t, result)
}

func TestPush_RegistryUnreachable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeMinimalOCILayout(t, filepath.Join(dir, "localpkg"))
	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "push-test"
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

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	cfg.Options.PlainHTTP = true
	result, err := Push(t.Context(), tarball.OutputPath, "localhost:0/test/bundle:v1.0.0", PushOptions{
		Config: cfg,
	})

	// The specific error message varies by OS and connection failure type
	// (e.g. "connection refused", "can't assign requested address").
	require.Error(t, err)
	assert.Nil(t, result)
}

func TestPushPackage_RoundTrip(t *testing.T) {
	t.Parallel()

	pkgDir := t.TempDir()
	writeMinimalOCILayout(t, filepath.Join(pkgDir, "oci"))

	dst, err := oraci.New(t.TempDir())
	require.NoError(t, err)

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := NewDefaultPusher().PushPackage(t.Context(), pkgDir, "example.com/test/pkg:1.0.0", PushOptions{
		Config:    cfg,
		PushHooks: pushTo(dst),
	})
	require.NoError(t, err)
	assert.Equal(t, "example.com/test/pkg:1.0.0", result.OCIReference)

	_, err = dst.Resolve(t.Context(), "1.0.0")
	require.NoError(t, err, "manifest should be present in store after push")
}

func TestPushPackage_EmptyPackageDir(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := NewDefaultPusher().PushPackage(t.Context(), "", "example.com/test/pkg:1.0.0", PushOptions{
		Config: cfg,
	})
	require.ErrorContains(t, err, "packageDir must not be empty")
	assert.Nil(t, result)
}

func TestPushPackage_EmptyOCIReference(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := NewDefaultPusher().PushPackage(t.Context(), t.TempDir(), "", PushOptions{
		Config: cfg,
	})
	require.ErrorContains(t, err, "ociReference must not be empty")
	assert.Nil(t, result)
}

func TestPushHooks_ModifyOrasSettings(t *testing.T) {
	t.Parallel()

	pkgDir := t.TempDir()
	writeMinimalOCILayout(t, filepath.Join(pkgDir, "oci"))

	dst, err := oraci.New(t.TempDir())
	require.NoError(t, err)

	hookCalled := false
	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := NewDefaultPusher().PushPackage(t.Context(), pkgDir, "example.com/test/pkg:1.0.0", PushOptions{
		Config: cfg,
		PushHooks: PushHooks{
			ToOrasTarget: func(context.Context, string, *PushOptions) (oras.Target, error) { return dst, nil },
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

func TestPushPackage_DoesNotModifySourceIndex(t *testing.T) {
	t.Parallel()

	pkgDir := t.TempDir()
	ociDir := filepath.Join(pkgDir, "oci")
	writeMinimalOCILayout(t, ociDir)

	// Read the original index.json.
	originalIndexPath := filepath.Join(ociDir, "index.json")
	originalIndexBytes, err := os.ReadFile(originalIndexPath)
	require.NoError(t, err)

	// Parse original to verify it has exactly one manifest (no tags).
	var originalIdx ociIndex
	require.NoError(t, json.Unmarshal(originalIndexBytes, &originalIdx))
	require.Len(t, originalIdx.Manifests, 1, "original index should have exactly one manifest")

	dst, err := oraci.New(t.TempDir())
	require.NoError(t, err)

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	_, err = NewDefaultPusher().PushPackage(t.Context(), pkgDir, "example.com/test/pkg:1.0.0", PushOptions{
		Config:    cfg,
		PushHooks: pushTo(dst),
	})
	require.NoError(t, err)

	// Read the index.json after push and verify it's unchanged.
	afterIndexBytes, err := os.ReadFile(originalIndexPath)
	require.NoError(t, err)
	assert.Equal(t, originalIndexBytes, afterIndexBytes, "source index.json should not be modified by PushPackage")

	// Verify the manifest is still in the store and unchanged.
	var afterIdx ociIndex
	require.NoError(t, json.Unmarshal(afterIndexBytes, &afterIdx))
	require.Len(t, afterIdx.Manifests, 1, "index should still have exactly one manifest after push")
}
