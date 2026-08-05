// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	oraci "oras.land/oras-go/v2/content/oci"
)

// pushTo returns PushHooks that inject target as the push destination via the
// ToOrasTarget seam, the same path production uses to resolve a registry.
func pushTo(target oras.Target) PushHooks {
	return PushHooks{
		ToOrasTarget: func(context.Context, string, *PushOptions) (oras.Target, error) { return target, nil },
	}
}

// createArchTestBundle creates a single-local-package bundle tarball for the
// given name/version/arch and returns the tarball path.
func createArchTestBundle(t *testing.T, name, version, arch string) string {
	t.Helper()
	dir := t.TempDir()
	writeMinimalOCILayout(t, filepath.Join(dir, "localpkg"))
	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, fmt.Appendf(nil, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "%s"
  version = "%s"
}
package "pkg1" {
  source = "localpkg"
  signature_verification { verify = false }
}
`, name, version), tmpFilePerm))

	tarball, err := Create(t.Context(), CreateOptions{
		Config:     newTestConfigWithArch(arch),
		BundleFile: bundleFile,
		Streams:    iostreams.New(nil, nil, io.Discard),
	})
	require.NoError(t, err)
	return tarball.OutputPath
}

// pushArchTestBundle pushes a bundle tarball to dst at ref via the ToOrasTarget seam.
func pushArchTestBundle(t *testing.T, dst oras.Target, ref, tarballPath string) {
	t.Helper()
	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	_, err := Push(t.Context(), tarballPath, ref, PushOptions{
		Config:    cfg,
		PushHooks: pushTo(dst),
	})
	require.NoError(t, err)
}

// fetchRootIndex resolves the tag on dst and returns the fetched index.
func fetchRootIndex(t *testing.T, dst oras.Target, tag string) ociIndex {
	t.Helper()
	desc, err := dst.Resolve(t.Context(), tag)
	require.NoError(t, err)
	data, err := content.FetchAll(t.Context(), dst, desc)
	require.NoError(t, err)
	var idx ociIndex
	require.NoError(t, json.Unmarshal(data, &idx))
	return idx
}

// fetchChildIndex fetches a child bundle index by its root-entry descriptor.
func fetchChildIndex(t *testing.T, dst oras.Target, entry ociManifest) ociIndex {
	t.Helper()
	d, err := parseDigest(entry.Digest)
	require.NoError(t, err)
	data, err := content.FetchAll(t.Context(), dst, ocispec.Descriptor{MediaType: entry.MediaType, Digest: d, Size: entry.Size})
	require.NoError(t, err)
	var idx ociIndex
	require.NoError(t, json.Unmarshal(data, &idx))
	return idx
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
  signature_verification { verify = false }
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
  signature_verification { verify = false }
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
  signature_verification { verify = false }
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

func TestPush_PublishesRootIndexRoutingToChild(t *testing.T) {
	t.Parallel()

	dst, err := oraci.New(t.TempDir())
	require.NoError(t, err)
	tarball := createArchTestBundle(t, "root-route", "1.0.0", "amd64")
	pushArchTestBundle(t, dst, "example.com/test/root-route:1.0.0", tarball)

	root := fetchRootIndex(t, dst, "1.0.0")
	assert.Empty(t, root.ArtifactType, "root index is a plain platform router")
	require.Len(t, root.Manifests, 1)

	entry := root.Manifests[0]
	assert.Equal(t, ocispec.MediaTypeImageIndex, entry.MediaType)
	assert.Equal(t, MediaTypeBundle, entry.ArtifactType)
	require.NotNil(t, entry.Platform)
	assert.Equal(t, "amd64", entry.Platform.Architecture)
	assert.Equal(t, oci.MultiOS, entry.Platform.OS)

	child := fetchChildIndex(t, dst, entry)
	assert.Equal(t, MediaTypeBundle, child.ArtifactType)
	assert.Equal(t, "amd64", child.Annotations[AnnotationBundleArchitecture])

	// The child index bytes in the registry match the tarball's index.json.
	entries := readTarZstEntries(t, tarball)
	assert.Equal(t, godigest.FromBytes(entries["oci/index.json"]).String(), entry.Digest,
		"registry child index must be byte-identical to the tarball index.json")
}

func TestPush_MergesSecondArchitectureIntoRootIndex(t *testing.T) {
	t.Parallel()

	dst, err := oraci.New(t.TempDir())
	require.NoError(t, err)
	ref := "example.com/test/multi:2.0.0"
	pushArchTestBundle(t, dst, ref, createArchTestBundle(t, "multi", "2.0.0", "amd64"))
	pushArchTestBundle(t, dst, ref, createArchTestBundle(t, "multi", "2.0.0", "arm64"))

	root := fetchRootIndex(t, dst, "2.0.0")
	require.Len(t, root.Manifests, 2, "root index must hold one entry per architecture")
	assert.Equal(t, "amd64", root.Manifests[0].Platform.Architecture)
	assert.Equal(t, "arm64", root.Manifests[1].Platform.Architecture)

	for _, entry := range root.Manifests {
		child := fetchChildIndex(t, dst, entry)
		assert.Equal(t, entry.Platform.Architecture, child.Annotations[AnnotationBundleArchitecture])
	}
}

func TestPush_ReplacesSameArchitectureEntry(t *testing.T) {
	t.Parallel()

	dst, err := oraci.New(t.TempDir())
	require.NoError(t, err)
	ref := "example.com/test/replace:3.0.0"
	pushArchTestBundle(t, dst, ref, createArchTestBundle(t, "replace", "3.0.0", "amd64"))
	before := fetchRootIndex(t, dst, "3.0.0")
	require.Len(t, before.Manifests, 1)

	// A re-push of the same architecture with different content replaces the
	// entry rather than appending a duplicate. (Different bundle name changes
	// the child digest while the tag stays the same.)
	pushArchTestBundle(t, dst, ref, createArchTestBundle(t, "replace-updated", "3.0.0", "amd64"))
	after := fetchRootIndex(t, dst, "3.0.0")
	require.Len(t, after.Manifests, 1, "same-arch re-push must replace, not append")
	assert.NotEqual(t, before.Manifests[0].Digest, after.Manifests[0].Digest)
	assert.Equal(t, "amd64", after.Manifests[0].Platform.Architecture)
}

func TestPush_DigestReferenceRejected(t *testing.T) {
	t.Parallel()

	dst, err := oraci.New(t.TempDir())
	require.NoError(t, err)
	tarball := createArchTestBundle(t, "digest-ref", "1.0.0", "amd64")

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	_, err = Push(t.Context(), tarball, "example.com/test/digest-ref@sha256:0000000000000000000000000000000000000000000000000000000000000000", PushOptions{
		Config:    cfg,
		PushHooks: pushTo(dst),
	})
	require.ErrorContains(t, err, "must be pushed to a tag reference")
}
