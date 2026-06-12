// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/mholt/archives"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

type fixtureBlob struct {
	title   string // org.opencontainers.image.title annotation (zarf.yaml/sig/checksums)
	content []byte
}

type fixturePkg struct {
	name  string // the bundle-level package name
	blobs []fixtureBlob
}

func zarfYAML(name, version string) []byte {
	return []byte("kind: ZarfPackageConfig\nmetadata:\n  name: " + name + "\n  version: " + version + "\n")
}

// writeFixtureBundle builds a minimal .tar.zst bundle on disk containing only
// the OCI blobs the prefetcher cares about: one image manifest per package plus
// that package's annotated metadata blobs. When manifestFirst is true each
// package's manifest is written ahead of its metadata blobs (the order the real
// writeTarball produces); when false the metadata blobs come first, which the
// single-pass prefetcher cannot resolve. Returns the tarball path and, indexed
// to match pkgs, each package's manifest digest (encoded, no algorithm prefix).
func writeFixtureBundle(t *testing.T, pkgs []fixturePkg, manifestFirst bool) (string, []string) {
	t.Helper()
	srcDir := t.TempDir()
	blobsDir := filepath.Join(srcDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobsDir, 0o755))

	fileMap := map[string]string{} // disk path -> name in archive
	var order []string             // archive names in the order we want them in the tar
	manifestHexes := make([]string, len(pkgs))

	writeBlob := func(content []byte) digest.Digest {
		d := digest.FromBytes(content)
		require.NoError(t, os.WriteFile(filepath.Join(blobsDir, d.Encoded()), content, 0o644))
		fileMap[filepath.Join(blobsDir, d.Encoded())] = config.BlobsDir + "/" + d.Encoded()
		return d
	}

	for i, p := range pkgs {
		var layers []ocispec.Descriptor
		var metaNames []string
		for _, b := range p.blobs {
			d := writeBlob(b.content)
			layers = append(layers, ocispec.Descriptor{
				Digest:      d,
				Size:        int64(len(b.content)),
				Annotations: map[string]string{ocispec.AnnotationTitle: b.title},
			})
			metaNames = append(metaNames, config.BlobsDir+"/"+d.Encoded())
		}

		manifestBytes, err := json.Marshal(oci.Manifest{Manifest: ocispec.Manifest{Layers: layers}})
		require.NoError(t, err)
		md := writeBlob(manifestBytes)
		manifestHexes[i] = md.Encoded()

		manifestName := config.BlobsDir + "/" + md.Encoded()
		if manifestFirst {
			order = append(order, manifestName)
			order = append(order, metaNames...)
		} else {
			order = append(order, metaNames...)
			order = append(order, manifestName)
		}
	}

	files, err := archives.FilesFromDisk(context.Background(), nil, fileMap)
	require.NoError(t, err)

	// Order the slice to control tar layout; CompressedArchive.Archive writes
	// entries in slice order.
	rank := make(map[string]int, len(order))
	for i, n := range order {
		rank[n] = i
	}
	sort.SliceStable(files, func(a, b int) bool {
		return rank[files[a].NameInArchive] < rank[files[b].NameInArchive]
	})

	tarPath := filepath.Join(t.TempDir(), "bundle.tar.zst")
	out, err := os.Create(tarPath)
	require.NoError(t, err)
	require.NoError(t, config.BundleArchiveFormat.Archive(context.Background(), out, files))
	require.NoError(t, out.Close())

	return tarPath, manifestHexes
}

func TestPrefetchPackageMetadata(t *testing.T) {
	ctx := context.Background()

	t.Run("captures all packages when the manifest precedes its metadata", func(t *testing.T) {
		pkgs := []fixturePkg{
			{name: "signed-pkg", blobs: []fixtureBlob{
				{title: layout.ZarfYAML, content: zarfYAML("internal-signed", "1.2.3")},
				{title: layout.Signature, content: []byte("fake-signature")},
				{title: config.ChecksumsTxt, content: []byte("fake-checksums")},
			}},
			{name: "unsigned-pkg", blobs: []fixtureBlob{
				{title: layout.ZarfYAML, content: zarfYAML("internal-unsigned", "4.5.6")},
				{title: config.ChecksumsTxt, content: []byte("more-checksums")},
			}},
		}
		tarPath, hexes := writeFixtureBundle(t, pkgs, true)

		tp := &tarballBundleProvider{src: tarPath}
		results, err := tp.prefetchPackageMetadata(ctx, []types.Package{
			{Name: "signed-pkg", Ref: "ghcr.io/x/signed@sha256:" + hexes[0]},
			{Name: "unsigned-pkg", Ref: "ghcr.io/x/unsigned@sha256:" + hexes[1]},
		}, t.TempDir())
		require.NoError(t, err)
		require.Len(t, results, 2)

		// The prefetcher only stages the metadata files on disk; getMetadata
		// loads/verifies them. Assert the right files landed in each pkg's dir.
		signed := results["signed-pkg"]
		require.NotNil(t, signed)
		require.FileExists(t, filepath.Join(signed.dirPath, layout.ZarfYAML))
		require.FileExists(t, filepath.Join(signed.dirPath, layout.Signature))
		require.FileExists(t, filepath.Join(signed.dirPath, layout.Checksums))

		unsigned := results["unsigned-pkg"]
		require.NotNil(t, unsigned)
		require.FileExists(t, filepath.Join(unsigned.dirPath, layout.ZarfYAML))
		require.FileExists(t, filepath.Join(unsigned.dirPath, layout.Checksums))
		require.NoFileExists(t, filepath.Join(unsigned.dirPath, layout.Signature))
	})

	t.Run("fails when metadata precedes its manifest in the stream", func(t *testing.T) {
		// This is the single-pass contract that writeTarball satisfies by
		// ordering each package's manifest before its metadata blobs. If the
		// metadata streams first, the prefetcher skips it (not yet known to be
		// needed) and never recovers it in one forward pass.
		pkgs := []fixturePkg{{name: "p", blobs: []fixtureBlob{
			{title: layout.ZarfYAML, content: zarfYAML("p", "1.0.0")},
			{title: config.ChecksumsTxt, content: []byte("c")},
		}}}
		tarPath, hexes := writeFixtureBundle(t, pkgs, false)

		tp := &tarballBundleProvider{src: tarPath}
		_, err := tp.prefetchPackageMetadata(ctx, []types.Package{
			{Name: "p", Ref: "ghcr.io/x/p@sha256:" + hexes[0]},
		}, t.TempDir())
		require.Error(t, err)
	})

	t.Run("rejects duplicate package names", func(t *testing.T) {
		pkgs := []fixturePkg{
			{name: "dup", blobs: []fixtureBlob{
				{title: layout.ZarfYAML, content: zarfYAML("a", "1.0.0")},
				{title: config.ChecksumsTxt, content: []byte("c1")},
			}},
			{name: "dup", blobs: []fixtureBlob{
				{title: layout.ZarfYAML, content: zarfYAML("b", "2.0.0")},
				{title: config.ChecksumsTxt, content: []byte("c2")},
			}},
		}
		tarPath, hexes := writeFixtureBundle(t, pkgs, true)

		tp := &tarballBundleProvider{src: tarPath}
		_, err := tp.prefetchPackageMetadata(ctx, []types.Package{
			{Name: "dup", Ref: "ghcr.io/x/a@sha256:" + hexes[0]},
			{Name: "dup", Ref: "ghcr.io/x/b@sha256:" + hexes[1]},
		}, t.TempDir())
		require.ErrorContains(t, err, "multiple packages named")
	})

	t.Run("rejects a ref without a manifest digest", func(t *testing.T) {
		tp := &tarballBundleProvider{src: "ignored-no-file-opened"}
		_, err := tp.prefetchPackageMetadata(ctx, []types.Package{
			{Name: "p", Ref: "ghcr.io/x/p:1.0.0"},
		}, t.TempDir())
		require.ErrorContains(t, err, "missing manifest digest")
	})

	t.Run("rejects a ref with a malformed manifest digest", func(t *testing.T) {
		for _, bad := range []string{
			"../../../etc/passwd", // path traversal
			"short",               // wrong length
			"zz5e8b1f6a4d3c2e9f0a1b2c3d4e5f60718293a4b5c6d7e8f90112233445566", // non-hex
		} {
			tp := &tarballBundleProvider{src: "ignored-no-file-opened"}
			_, err := tp.prefetchPackageMetadata(ctx, []types.Package{
				{Name: "p", Ref: "ghcr.io/x/p@sha256:" + bad},
			}, t.TempDir())
			require.ErrorContains(t, err, "invalid manifest digest", "digest %q should be rejected", bad)
		}
	})

	t.Run("errors when a needed blob exceeds the size cap", func(t *testing.T) {
		orig := maxPrefetchBlobBytes
		maxPrefetchBlobBytes = 1024
		t.Cleanup(func() { maxPrefetchBlobBytes = orig })

		// Manifest stays under the cap and parses; the oversized zarf.yaml blob
		// trips the overflow guard.
		pkgs := []fixturePkg{{name: "p", blobs: []fixtureBlob{
			{title: layout.ZarfYAML, content: bytes.Repeat([]byte("x"), 2048)},
			{title: config.ChecksumsTxt, content: []byte("c")},
		}}}
		tarPath, hexes := writeFixtureBundle(t, pkgs, true)

		tp := &tarballBundleProvider{src: tarPath}
		_, err := tp.prefetchPackageMetadata(ctx, []types.Package{
			{Name: "p", Ref: "ghcr.io/x/p@sha256:" + hexes[0]},
		}, t.TempDir())
		require.ErrorContains(t, err, "exceeded")
	})

	t.Run("errors when a package manifest is absent from the bundle", func(t *testing.T) {
		pkgs := []fixturePkg{{name: "present", blobs: []fixtureBlob{
			{title: layout.ZarfYAML, content: zarfYAML("present", "1.0.0")},
			{title: config.ChecksumsTxt, content: []byte("c")},
		}}}
		tarPath, _ := writeFixtureBundle(t, pkgs, true)

		ghost := digest.FromBytes([]byte("ghost-manifest")).Encoded()
		tp := &tarballBundleProvider{src: tarPath}
		_, err := tp.prefetchPackageMetadata(ctx, []types.Package{
			{Name: "ghost", Ref: "ghcr.io/x/ghost@sha256:" + ghost},
		}, t.TempDir())
		require.ErrorContains(t, err, "not found in bundle")
	})
}
