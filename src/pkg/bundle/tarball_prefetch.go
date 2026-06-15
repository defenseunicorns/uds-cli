// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/mholt/archives"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// pkgPrefetchResult holds the on-disk location of the small metadata files
// (zarf.yaml/signature/checksums) prefetched for a single bundled Zarf package.
// getMetadata loads and verifies the package from this directory.
type pkgPrefetchResult struct {
	// dirPath is the directory containing zarf.yaml/signature/checksums on disk.
	dirPath string
}

// maxPrefetchBlobBytes caps in-memory capture of any single blob the prefetcher
// considers "needed." Today this only ever includes package image manifests
// (small JSON), zarf.yaml, signature.sig, and checksums.txt — all <1 MB in
// practice. The bound is defensive: if a future contributor adds a new title
// to the switch in the handler below, a hostile or oversized blob can't OOM
// the inspect process before we notice.
//
// This is a var rather than a const so tests can shrink it to exercise the
// overflow guard without writing multi-MB fixtures.
var maxPrefetchBlobBytes int64 = 32 << 20 // 32 MiB

// prefetchPackageMetadata streams the bundle tarball ONCE and pulls every
// package's zarf.yaml, signature, and checksums.txt blob out of it.
//
// The naive path in src/pkg/sources/tarball.go LoadPackageMetadata opens the
// bundle twice per package (once for the package manifest, once for the
// referenced metadata blobs). For a bundle with N packages that's roughly N*2
// full zstd decompressions of a multi-GB tarball — the dominant cost of
// `uds inspect`. Here we collapse it to a single pass: we drive a state
// machine that knows about the package manifest digests up front, parses each
// manifest as it streams by to learn the digests of its metadata layers, and
// returns fs.SkipAll the moment every parser is satisfied. For new bundles
// where metadata is at the front of the tar (see writeTarball priority list),
// we typically exit after decompressing only the first few MB.
//
// Result blobs are written to per-package directories under dstDir so the
// existing signature-verification path can run against them unchanged.
func (tp *tarballBundleProvider) prefetchPackageMetadata(ctx context.Context, packages []types.Package, dstDir string) (map[string]*pkgPrefetchResult, error) {
	// Build the up-front "things we want" set from each Package.Ref. The
	// digest after @sha256: is the package manifest blob digest (uds-cli
	// appends it during create).
	type pkgEntry struct {
		pkg            types.Package
		manifestDigest string
	}
	entries := make([]pkgEntry, 0, len(packages))
	for _, pkg := range packages {
		idx := strings.LastIndex(pkg.Ref, "@sha256:")
		if idx < 0 {
			return nil, fmt.Errorf("package %q ref %q missing manifest digest", pkg.Name, pkg.Ref)
		}
		manifestDigest := pkg.Ref[idx+len("@sha256:"):]
		// pkg.Ref originates from bundle metadata. This digest is used both to
		// match tar entry names and to build on-disk paths (pkgmeta-<digest>),
		// so validate it's a well-formed sha256 before use — otherwise a
		// hostile ref could smuggle path separators and cause traversal.
		if err := digest.NewDigestFromEncoded(digest.SHA256, manifestDigest).Validate(); err != nil {
			return nil, fmt.Errorf("package %q has invalid manifest digest %q: %w", pkg.Name, manifestDigest, err)
		}
		entries = append(entries, pkgEntry{pkg: pkg, manifestDigest: manifestDigest})
	}

	// State that the streaming handler reads & mutates:
	// - needed: blob digests we still want
	// - captured: digest -> bytes for every needed blob we've already read
	// - parsedManifests: digest -> parsed package manifest (used to discover
	//   the per-package metadata layer digests on the fly)
	needed := make(map[string]bool, len(entries))
	for _, e := range entries {
		needed[e.manifestDigest] = true
	}
	isPackageManifest := make(map[string]bool, len(entries))
	for _, e := range entries {
		isPackageManifest[e.manifestDigest] = true
	}
	captured := make(map[string][]byte)
	parsedManifests := make(map[string]*oci.Manifest)

	handler := func(_ context.Context, file archives.FileInfo) error {
		// Only blobs are interesting; bundle layout puts everything under blobs/sha256/.
		if !strings.HasPrefix(file.NameInArchive, config.BlobsDir+"/") {
			return nil
		}
		digest := strings.TrimPrefix(file.NameInArchive, config.BlobsDir+"/")
		if !needed[digest] {
			return nil
		}
		delete(needed, digest)

		stream, err := file.Open()
		if err != nil {
			return err
		}
		defer stream.Close()

		// Bounded read: see maxPrefetchBlobBytes. +1 so we can detect overflow
		// rather than silently truncate.
		buf, err := io.ReadAll(io.LimitReader(stream, maxPrefetchBlobBytes+1))
		if err != nil {
			return err
		}
		if int64(len(buf)) > maxPrefetchBlobBytes {
			return fmt.Errorf("prefetch blob %s exceeded %d byte cap", digest, maxPrefetchBlobBytes)
		}
		captured[digest] = buf

		// If this blob is a package manifest, parse it now so we can discover
		// the metadata-layer digests we still need to fetch from this same
		// stream. Order is unpredictable in pre-priority bundles; we may have
		// already passed (and ignored) some layer blob — that's why we keep
		// streaming until everything is captured rather than bailing here.
		if isPackageManifest[digest] {
			var manifest oci.Manifest
			if err := json.Unmarshal(buf, &manifest); err != nil {
				return fmt.Errorf("parsing package manifest %s: %w", digest, err)
			}
			parsedManifests[digest] = &manifest
			for _, layer := range manifest.Layers {
				switch layer.Annotations[ocispec.AnnotationTitle] {
				case layout.ZarfYAML, layout.Signature, layout.Bundle, config.ChecksumsTxt:
					ld := layer.Digest.Encoded()
					if _, already := captured[ld]; !already {
						needed[ld] = true
					}
				}
			}
		}

		// Stop iterating once every package manifest has been parsed AND
		// every transitively-needed blob has been captured. For priority-
		// ordered bundles this happens in the first few MB; for old bundles
		// it may take a full drain to EOF — still a single drain rather than
		// N*2 drains.
		if len(needed) == 0 && len(parsedManifests) == len(entries) {
			return fs.SkipAll
		}
		return nil
	}

	srcFile, err := os.Open(tp.src)
	if err != nil {
		return nil, err
	}
	defer srcFile.Close()
	if err := config.BundleArchiveFormat.Extract(ctx, srcFile, handler); err != nil {
		return nil, err
	}

	// Resolve each package: write its metadata files to per-package subdirs.
	// We key the on-disk dir on the manifest digest so two packages with the
	// same logical name (but different versions) don't collide on disk.
	results := make(map[string]*pkgPrefetchResult, len(entries))
	for _, e := range entries {
		manifest, ok := parsedManifests[e.manifestDigest]
		if !ok {
			return nil, fmt.Errorf("package manifest %s for %q not found in bundle", e.manifestDigest, e.pkg.Name)
		}
		pkgDir := filepath.Join(dstDir, "pkgmeta-"+e.manifestDigest)
		if err := os.MkdirAll(pkgDir, 0o700); err != nil {
			return nil, err
		}

		var zarfYAMLPath string
		for _, layer := range manifest.Layers {
			data, ok := captured[layer.Digest.Encoded()]
			if !ok {
				continue
			}
			var dst string
			switch layer.Annotations[ocispec.AnnotationTitle] {
			case layout.ZarfYAML:
				dst = filepath.Join(pkgDir, layout.ZarfYAML)
				zarfYAMLPath = dst
			case layout.Signature:
				dst = filepath.Join(pkgDir, layout.Signature)
			case layout.Bundle:
				// keyless per-package signature; the slow path in
				// sources/tarball.go stages this too, and LoadPackageFromDir
				// needs it to verify keyless-signed packages.
				dst = filepath.Join(pkgDir, layout.Bundle)
			case config.ChecksumsTxt:
				dst = filepath.Join(pkgDir, layout.Checksums)
			default:
				continue
			}
			if err := os.WriteFile(dst, data, helpers.ReadWriteUser); err != nil {
				return nil, err
			}
		}
		if zarfYAMLPath == "" {
			return nil, fmt.Errorf("zarf.yaml not found for package %q in bundle", e.pkg.Name)
		}

		// The cache is keyed by package name (matching inspect.go's lookup). If
		// two packages share a name, the second would silently overwrite the
		// first and getMetadata would return the wrong metadata. On-disk dirs are
		// per-manifest-digest so they don't collide, but the result map would.
		if _, exists := results[e.pkg.Name]; exists {
			return nil, fmt.Errorf("bundle contains multiple packages named %q; cannot prefetch metadata unambiguously", e.pkg.Name)
		}
		results[e.pkg.Name] = &pkgPrefetchResult{dirPath: pkgDir}
	}

	return results, nil
}
