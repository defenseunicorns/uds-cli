// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/mholt/archives"
	godigest "github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	zarfarchive "github.com/zarf-dev/zarf/src/pkg/archive"
	oras "oras.land/oras-go/v2"
)

// CreateOptions contains inputs for the test bundle creator.
type CreateOptions struct {
	Config     *Config
	BundleFile string
	Streams    iostreams.IOStreams
}

// CreateResult reports the artifact path produced by the test bundle creator.
type CreateResult struct {
	OutputPath string
}

// newTestConfig returns test configuration for the runtime architecture.
func newTestConfig() *Config {
	return newTestConfigWithArch(runtime.GOARCH)
}

// newTestConfigWithArch returns test configuration for a specific architecture.
func newTestConfigWithArch(arch string) *Config {
	opts := &ConfigOptions{Architecture: arch, TmpDir: os.TempDir(), Concurrency: 10}
	return &Config{Global: &GlobalOptions{}, Options: opts}
}

// writeTestBlob writes content-addressed test data and returns its encoded digest.
func writeTestBlob(t *testing.T, blobDir string, data []byte) string {
	t.Helper()
	sum := sha256.Sum256(data)
	hexDigest := hex.EncodeToString(sum[:])
	require.NoError(t, os.WriteFile(filepath.Join(blobDir, hexDigest), data, tmpFilePerm))
	return hexDigest
}

// writeMinimalOCILayout creates a valid minimal OCI layout for tests.
func writeMinimalOCILayout(t *testing.T, ociDir string) {
	t.Helper()
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))
	require.NoError(t, os.WriteFile(filepath.Join(ociDir, "oci-layout"), []byte("{\"imageLayoutVersion\":\"1.0.0\"}\n"), tmpFilePerm))

	config := []byte("{}")
	configHex := writeTestBlob(t, blobDir, config)
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: "application/vnd.oci.empty.v1+json",
			Digest:    godigest.NewDigestFromEncoded(godigest.SHA256, configHex),
			Size:      int64(len(config)),
		},
		Layers: []ocispec.Descriptor{},
	}
	manifestBytes, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestHex := writeTestBlob(t, blobDir, manifestBytes)
	require.NoError(t, WriteIndex(filepath.Join(ociDir, "index.json"), &ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{{
			MediaType: ocispec.MediaTypeImageManifest,
			Digest:    godigest.NewDigestFromEncoded(godigest.SHA256, manifestHex),
			Size:      int64(len(manifestBytes)),
		}},
	}))
}

func writeMinimalZarfPackage(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, tempDirPerm))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zarf.yaml"), []byte("metadata:\n  name: test\n  version: 0.0.1\ncomponents: []\n"), tmpFilePerm))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "checksums.txt"), nil, tmpFilePerm))
}

// writeBundleOCILayout creates a test bundle OCI layout and returns its path.
func writeBundleOCILayout(t *testing.T, name, version string) string {
	t.Helper()
	root := t.TempDir()
	hclData := fmt.Appendf(nil, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = %q
  version = %q
}
`, name, version)
	writeBundleLayout(t, filepath.Join(root, "oci"), hclData, runtime.GOARCH)
	return filepath.Join(root, "oci")
}

// writeBundleLayout writes a bundle definition into an OCI layout.
func writeBundleLayout(t *testing.T, ociDir string, hclData []byte, arch string) {
	t.Helper()
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))
	require.NoError(t, os.WriteFile(filepath.Join(ociDir, "oci-layout"), []byte("{\"imageLayoutVersion\":\"1.0.0\"}\n"), tmpFilePerm))

	config := []byte("{}")
	configHex := writeTestBlob(t, blobDir, config)
	hclHex := writeTestBlob(t, blobDir, hclData)
	manifest := ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: MediaTypeBundleDefinition,
		Config: ocispec.Descriptor{
			MediaType: "application/vnd.oci.empty.v1+json",
			Digest:    godigest.NewDigestFromEncoded(godigest.SHA256, configHex),
			Size:      int64(len(config)),
		},
		Layers: []ocispec.Descriptor{{
			MediaType: MediaTypeBundleHCL,
			Digest:    godigest.NewDigestFromEncoded(godigest.SHA256, hclHex),
			Size:      int64(len(hclData)),
			Annotations: map[string]string{
				ocispec.AnnotationTitle: bundleinternal.BundleFileName,
			},
		}},
	}
	manifestBytes, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestHex := writeTestBlob(t, blobDir, manifestBytes)
	idx := NewBundleIndex([]ocispec.Descriptor{{
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: MediaTypeBundleDefinition,
		Digest:       godigest.NewDigestFromEncoded(godigest.SHA256, manifestHex),
		Size:         int64(len(manifestBytes)),
	}}, arch)
	require.NoError(t, WriteIndex(filepath.Join(ociDir, "index.json"), idx))
}

// bundleNameFromDefinitionLayer derives an artifact name from bundle HCL in a layout.
func bundleNameFromDefinitionLayer(ctx context.Context, streams iostreams.IOStreams, ociDir string, idx any, arch string) (string, error) {
	var digestHex string
	switch typed := idx.(type) {
	case ocispec.Index:
		entry, _, err := FindBundleDefinition(typed)
		if err != nil {
			return "", err
		}
		digestHex = entry.Digest.Hex()
	default:
		return "", fmt.Errorf("unsupported index type %T", idx)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(ociDir, "blobs", "sha256", digestHex))
	if err != nil {
		return "", err
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return "", err
	}
	var hclDigest string
	for _, layer := range manifest.Layers {
		if layer.MediaType == MediaTypeBundleHCL {
			hclDigest = layer.Digest.Hex()
			break
		}
	}
	if hclDigest == "" {
		return "", fmt.Errorf("bundle HCL layer not found in config manifest")
	}
	hclData, err := os.ReadFile(filepath.Join(ociDir, "blobs", "sha256", hclDigest))
	if err != nil {
		return "", err
	}
	bundle, err := bundleinternal.NewHCLParser(arch, streams).ParseBundleBytes(ctx, hclData)
	if err != nil {
		return "", err
	}
	if arch == "" {
		arch = runtime.GOARCH
	}
	return fmt.Sprintf("uds-bundle-%s-%s-%s.tar.zst", bundle.Metadata.Name, arch, bundle.Metadata.Version), nil
}

// writeTarZst archives a test directory as tar.zst.
func writeTarZst(ctx context.Context, _ iostreams.IOStreams, dst, srcDir string) error {
	files, err := archives.FilesFromDisk(ctx, nil, map[string]string{srcDir + string(filepath.Separator): ""})
	if err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	ca := archives.CompressedArchive{Archival: archives.Tar{}, Compression: archives.Zstd{}}
	archiveErr := ca.Archive(ctx, f, files)
	return errors.Join(archiveErr, f.Close())
}

// createBundleArchive packages a test OCI layout as a bundle artifact.
func createBundleArchive(ctx context.Context, streams iostreams.IOStreams, ociDir, targetDir string, idx ocispec.Index, arch string) (string, error) {
	name, err := bundleNameFromDefinitionLayer(ctx, streams, ociDir, idx, arch)
	if err != nil {
		return "", err
	}
	outPath := filepath.Join(targetDir, name)
	if err := writeTarZst(ctx, streams, outPath, filepath.Dir(ociDir)); err != nil {
		return "", err
	}
	return outPath, nil
}

// Pull invokes the default puller with test archive creation hooks.
func Pull(ctx context.Context, ref, targetDir string, opts PullOptions) (*PullResult, error) {
	if opts.PullHooks.CreateBundleArchive == nil {
		opts.PullHooks.CreateBundleArchive = createBundleArchive
	}
	return NewDefaultPuller().PullBundle(ctx, ref, targetDir, opts)
}

// Push extracts a test artifact and invokes the default pusher.
func Push(ctx context.Context, tarball, ref string, opts PushOptions) (*PushResult, error) {
	tmp, err := os.MkdirTemp(opts.Config.Options.TmpDir, "oci-push-test-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := zarfarchive.Decompress(ctx, tarball, tmp, zarfarchive.DecompressOpts{OverwriteExisting: true}); err != nil {
		return nil, fmt.Errorf("extracting bundle: %w", err)
	}
	return NewDefaultPusher().PushBundle(ctx, tmp, ref, opts)
}

// Create assembles a minimal test bundle artifact.
func Create(ctx context.Context, opts CreateOptions) (*CreateResult, error) {
	hclData, err := os.ReadFile(opts.BundleFile)
	if err != nil {
		return nil, err
	}
	bundle, err := bundleinternal.NewHCLParser(opts.Config.Options.Architecture, opts.Streams).ParseBundleBytes(ctx, hclData)
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp(opts.Config.Options.TmpDir, "oci-create-test-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(root) }()
	blobDir := filepath.Join(root, "oci", "blobs", "sha256")
	if err := os.MkdirAll(blobDir, tempDirPerm); err != nil {
		return nil, err
	}
	// The fixture builder uses testing assertions, so construct the same layout inline.
	config := []byte("{}")
	configDigest := sha256.Sum256(config)
	hclDigest := sha256.Sum256(hclData)
	if err := os.WriteFile(filepath.Join(blobDir, hex.EncodeToString(configDigest[:])), config, tmpFilePerm); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(blobDir, hex.EncodeToString(hclDigest[:])), hclData, tmpFilePerm); err != nil {
		return nil, err
	}
	manifest := ocispec.Manifest{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageManifest, ArtifactType: MediaTypeBundleDefinition,
		Config: ocispec.Descriptor{MediaType: "application/vnd.oci.empty.v1+json", Digest: godigest.FromBytes(config), Size: int64(len(config))},
		Layers: []ocispec.Descriptor{{MediaType: MediaTypeBundleHCL, Digest: godigest.FromBytes(hclData), Size: int64(len(hclData))}}}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	manifestDigest := sha256.Sum256(manifestBytes)
	if err := os.WriteFile(filepath.Join(blobDir, hex.EncodeToString(manifestDigest[:])), manifestBytes, tmpFilePerm); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(root, "oci", "oci-layout"), []byte("{\"imageLayoutVersion\":\"1.0.0\"}\n"), tmpFilePerm); err != nil {
		return nil, err
	}
	idx := NewBundleIndex([]ocispec.Descriptor{{MediaType: ocispec.MediaTypeImageManifest, ArtifactType: MediaTypeBundleDefinition, Digest: godigest.NewDigestFromEncoded(godigest.SHA256, hex.EncodeToString(manifestDigest[:])), Size: int64(len(manifestBytes))}}, opts.Config.Options.Architecture)
	if err := WriteIndex(filepath.Join(root, "oci", "index.json"), idx); err != nil {
		return nil, err
	}
	out := filepath.Join(filepath.Dir(opts.BundleFile), fmt.Sprintf("uds-bundle-%s-%s-%s.tar.zst", bundle.Metadata.Name, opts.Config.Options.Architecture, bundle.Metadata.Version))
	if err := writeTarZst(ctx, opts.Streams, out, root); err != nil {
		return nil, err
	}
	return &CreateResult{OutputPath: out}, nil
}

// pushTo returns hooks that direct pushes to an in-memory test target.
func pushTo(dst oras.Target) PushHooks {
	return PushHooks{ToOrasTarget: func(context.Context, string, *PushOptions) (oras.Target, error) { return dst, nil }}
}

// createArchTestBundle creates a bundle artifact for a selected architecture.
func createArchTestBundle(t *testing.T, name, version, arch string) string {
	t.Helper()
	dir := t.TempDir()
	hclData := fmt.Appendf(nil, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = %q
  version = %q
}
`, name, version)
	writeBundleLayout(t, filepath.Join(dir, "oci"), hclData, arch)
	tarball := filepath.Join(t.TempDir(), "bundle.tar.zst")
	require.NoError(t, writeTarZst(t.Context(), iostreams.IOStreams{}, tarball, dir))
	return tarball
}

// pushArchTestBundle pushes an architecture-specific artifact to a test target.
func pushArchTestBundle(t *testing.T, dst oras.Target, ref, tarball string) {
	t.Helper()
	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	_, err := Push(t.Context(), tarball, ref, PushOptions{Config: cfg, PushHooks: pushTo(dst)})
	require.NoError(t, err)
}

// readTarZstEntries returns the files contained in a test artifact.
func readTarZstEntries(t *testing.T, tarball string) map[string][]byte {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, zarfarchive.Decompress(t.Context(), tarball, dir, zarfarchive.DecompressOpts{OverwriteExisting: true}))
	entries := map[string][]byte{}
	require.NoError(t, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err == nil {
			entries[filepath.ToSlash(rel)] = data
		}
		return err
	}))
	return entries
}
