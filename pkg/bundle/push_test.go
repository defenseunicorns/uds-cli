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
	oraci "oras.land/oras-go/v2/content/oci"
)

func TestPush_NoOCILayout(t *testing.T) {
	t.Parallel()

	// Build a tar.zst with no oci/ directory at all, simulating a v0 bundle.
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "some-file.txt"), []byte("not a bundle"), tmpFilePerm))

	tarball := filepath.Join(t.TempDir(), "v0-bundle.tar.zst")
	require.NoError(t, writeTarZst(context.Background(), iostreams.IOStreams{}, tarball, srcDir))

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := Push(context.Background(), tarball, "example.com/test/v0-bundle:v1.0.0", PushOptions{
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
	require.NoError(t, writeTarZst(context.Background(), iostreams.IOStreams{}, tarball, srcDir))

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := Push(context.Background(), tarball, "example.com/test/not-a-bundle:v1.0.0", PushOptions{
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

	tarball, err := Create(context.Background(), CreateOptions{
		Config:     newTestConfig(),
		BundleFile: bundleFile,
		Streams:    iostreams.IOStreams{ErrOut: os.Stderr},
	})
	require.NoError(t, err)

	dst, err := oraci.New(t.TempDir())
	require.NoError(t, err)

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := Push(context.Background(), tarball.OutputPath, "example.com/test/push-test:1.0.0", PushOptions{
		Config:     cfg,
		remoteRepo: dst,
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
	result, err := Push(context.Background(), "/nonexistent/bundle.tar.zst", "example.com/test/bundle:v1.0.0", PushOptions{
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
	tarball, err := Create(context.Background(), CreateOptions{
		Config:     newTestConfig(),
		BundleFile: bundleFile,
		Streams:    iostreams.IOStreams{ErrOut: os.Stderr},
	})
	require.NoError(t, err)

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	result, err := Push(context.Background(), tarball.OutputPath, ":::invalid:::", PushOptions{
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
	tarball, err := Create(context.Background(), CreateOptions{
		Config:     newTestConfig(),
		BundleFile: bundleFile,
		Streams:    iostreams.IOStreams{ErrOut: os.Stderr},
	})
	require.NoError(t, err)

	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	cfg.Options.PlainHTTP = true
	result, err := Push(context.Background(), tarball.OutputPath, "localhost:0/test/bundle:v1.0.0", PushOptions{
		Config: cfg,
	})

	// The specific error message varies by OS and connection failure type
	// (e.g. "connection refused", "can't assign requested address").
	require.Error(t, err)
	assert.Nil(t, result)
}
