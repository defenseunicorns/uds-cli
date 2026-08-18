// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

var _ PackageLayoutLoader = (*ExtractedArtifactPackageLayoutLoader)(nil)

// LoadPackageLayout stages indexed OCI layers into dstDir.
func (l *ExtractedArtifactPackageLayoutLoader) LoadPackageLayout(ctx context.Context, pkg *Package, dstDir string, opts LoadOptions) (*layout.PackageLayout, bool, error) {
	s := opts.Streams
	s.Debug("loading package layout", "name", pkg.Name, "dir", dstDir)
	key := pkg.Name
	if udsoci.IsOCIReference(pkg.Source) {
		key = udsoci.TrimScheme(pkg.Source)
	}
	descriptor, ok := l.PackageManifests[key]
	if !ok {
		if digestValue, found := l.PackageDigests[key]; found {
			digest, err := godigest.Parse(digestValue)
			if err != nil {
				return nil, false, fmt.Errorf("invalid manifest digest for package %q: %w", pkg.Name, err)
			}
			blobPath := filepath.Join(l.OCIDir, ocispec.ImageBlobsDir, digest.Algorithm().String(), digest.Encoded())
			info, err := os.Stat(blobPath)
			if err != nil {
				return nil, false, fmt.Errorf("stat manifest for package %q: %w", pkg.Name, err)
			}
			descriptor = ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest, Size: info.Size()}
			ok = true
		}
	}
	if !ok {
		keys := make([]string, 0, len(l.PackageManifests)+len(l.PackageDigests))
		for k := range l.PackageManifests {
			keys = append(keys, k)
		}
		for k := range l.PackageDigests {
			keys = append(keys, k)
		}
		return nil, false, fmt.Errorf("package %q (source %q) not found in bundle artifact index; available: %v", pkg.Name, pkg.Source, keys)
	}

	blobDir := filepath.Join(l.OCIDir, "blobs", "sha256")
	store, err := udsoci.OpenReadOnlyStore(l.OCIDir)
	if err != nil {
		return nil, false, fmt.Errorf("opening OCI layout for package %q: %w", pkg.Name, err)
	}
	manifestData, err := udsoci.FetchBytes(ctx, store, descriptor)
	if err != nil {
		return nil, false, fmt.Errorf("reading manifest for package %q: %w", pkg.Name, err)
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, false, fmt.Errorf("parsing manifest for package %q: %w", pkg.Name, err)
	}

	cleanDstDir, err := filepath.Abs(filepath.Clean(dstDir))
	if err != nil {
		return nil, false, fmt.Errorf("resolving destination directory: %w", err)
	}
	for _, layer := range manifest.Layers {
		title := layer.Annotations[ocispec.AnnotationTitle]
		if title == "" {
			return nil, false, fmt.Errorf("manifest for package %q missing title annotation on layer with digest %q", pkg.Name, layer.Digest)
		}
		src := filepath.Join(blobDir, layer.Digest.Hex())
		dst, err := safeLayerDestinationPath(cleanDstDir, dstDir, title)
		if err != nil {
			return nil, false, err
		}
		if err := os.MkdirAll(filepath.Dir(dst), tempDirPerm); err != nil {
			return nil, false, fmt.Errorf("creating dir for layer %q: %w", title, err)
		}
		if err := stagePackageLayer(ctx, src, dst, title); err != nil {
			return nil, false, fmt.Errorf("staging layer %q for package %q: %w", title, pkg.Name, err)
		}
	}

	filter := BuildComponentFilter(pkg.OptionalComponents)
	// OCI-blob-staged packages always use IsPartial: true — the bundle stores only the
	// layers ingested at create time; checksums.txt may reference filtered-out blobs.
	pkgLayout, err := layout.LoadFromDir(ctx, dstDir, artifactPackageLayoutOptions(filter, true))
	if err != nil {
		return nil, false, fmt.Errorf("loading package layout for %q: %w", pkg.Name, err)
	}
	return pkgLayout, true, nil
}

func artifactPackageLayoutOptions(filter filters.ComponentFilterStrategy, isPartial bool) layout.PackageLayoutOptions {
	return layout.PackageLayoutOptions{
		Filter:               filter,
		IsPartial:            isPartial,
		VerificationStrategy: layout.VerifyNever, // Non-create options do not verify underlying Zarf packages
	}
}

var _ PackageLayoutLoader = (*SourcePackageLayoutLoader)(nil)

// LoadPackageLayout pulls a package source into a deployable layout.
func (l *SourcePackageLayoutLoader) LoadPackageLayout(ctx context.Context, pkg *Package, dstDir string, opts LoadOptions) (*layout.PackageLayout, bool, error) {
	s := opts.Streams
	s.Info("pulling package", "source", pkg.Source)
	source := NewPackageSource(pkg.Source, l.configOpts, l.bundleDir, opts.Streams)
	filter := BuildComponentFilter(pkg.OptionalComponents)
	pkgLayout, err := source.PullFiltered(ctx, dstDir, layout.PackageLayoutOptions{
		Filter:               filter,
		IsPartial:            opts.IsPartial,
		VerificationStrategy: layout.VerifyNever,
	})
	if err != nil {
		return nil, false, fmt.Errorf("failed to load package %q from %s: %w", pkg.Name, pkg.Source, err)
	}
	advisoryVerifyPackage(ctx, pkgLayout, pkg, l.configOpts.TmpDir, s)
	return pkgLayout, opts.IsPartial, nil
}

func stagePackageLayer(ctx context.Context, src, dst, title string) error {
	// Zarf adds image-name annotations to this index during deploy. Copy it so
	// read-only OCI blobs remain immutable and the staged index is writable.
	if filepath.ToSlash(title) == "images/index.json" {
		return copyFileContents(ctx, src, dst)
	}
	return linkOrCopy(ctx, src, dst)
}

// linkOrCopy creates a hard link from src to dst; on cross-device link errors it falls back to a full copy.
// linkOrCopy links or copies a file from src to dst. When a hard link succeeds,
// the destination shares an inode with the source and must be treated as read-only
// since mutations would affect both. The OCI blobs written by this function are
// immutable and digest-addressed, and callers read and decompress them into
// separate temporary directories before use, so read-only access is safe.
func linkOrCopy(ctx context.Context, src, dst string) error {
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	return copyFileContents(ctx, src, dst)
}

// copyFileContents copies a file atomically while observing context cancellation.
func copyFileContents(ctx context.Context, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	// Write to a temp file first; rename to dst only on success to prevent partial files.
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, tmpFilePerm)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, &ctxReader{ctx: ctx, r: in})
	closeErr := out.Close()

	if copyErr != nil || ctx.Err() != nil {
		_ = os.Remove(tmp)
		if copyErr != nil {
			return copyErr
		}
		return ctx.Err()
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, dst)
}

// safeLayerDestinationPath resolves a layer title without allowing directory escape.
func safeLayerDestinationPath(cleanDstDir, dstDir, title string) (string, error) {
	dst := filepath.Join(dstDir, filepath.FromSlash(title))

	cleanDst, err := filepath.Abs(filepath.Clean(dst))
	if err != nil {
		return "", fmt.Errorf("resolving layer title %q: %w", title, err)
	}
	rel, err := filepath.Rel(cleanDstDir, cleanDst)
	if err != nil {
		return "", fmt.Errorf("checking layer title %q: %w", title, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("layer title %q escapes destination directory", title)
	}

	return dst, nil
}

// Read stops reads when the associated context is canceled.
func (r *ctxReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
