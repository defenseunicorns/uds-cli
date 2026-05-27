// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// ExtractedArtifactPackageLayoutLoader reads package OCI blobs from an extracted bundle artifact workspace.
type ExtractedArtifactPackageLayoutLoader struct {
	// OCIDir is <workspace>/oci — the extracted OCI image layout root.
	OCIDir string
	// PackageDigests maps source identifier → manifest digest ("sha256:...").
	// Keys match what the create pipeline writes as org.opencontainers.image.ref.name:
	// TrimScheme(source) for OCI sources, pkg.Name for local sources.
	PackageDigests map[string]string
}

var _ PackageLayoutLoader = (*ExtractedArtifactPackageLayoutLoader)(nil)

// LoadPackageLayout stages the package's OCI layers into dstDir, which must already exist.
func (l *ExtractedArtifactPackageLayoutLoader) LoadPackageLayout(ctx context.Context, pkg *Package, dstDir string) (*layout.PackageLayout, error) {
	slog.Debug("loading package from extracted bundle artifact", "name", pkg.Name, "dir", dstDir)
	key := pkg.Name
	if IsOCIReference(pkg.Source) {
		key = TrimScheme(pkg.Source)
	}
	digest, ok := l.PackageDigests[key]
	if !ok {
		keys := make([]string, 0, len(l.PackageDigests))
		for k := range l.PackageDigests {
			keys = append(keys, k)
		}
		return nil, fmt.Errorf("package %q (source %q) not found in bundle artifact index; available: %v", pkg.Name, pkg.Source, keys)
	}

	blobDir := filepath.Join(l.OCIDir, "blobs", "sha256")
	hex := strings.TrimPrefix(digest, "sha256:")
	manifestData, err := os.ReadFile(filepath.Join(blobDir, hex))
	if err != nil {
		return nil, fmt.Errorf("reading manifest for package %q: %w", pkg.Name, err)
	}
	var manifest ociImageManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest for package %q: %w", pkg.Name, err)
	}

	for _, layer := range manifest.Layers {
		title := layer.Annotations[ocispec.AnnotationTitle]
		if title == "" {
			return nil, fmt.Errorf("manifest for package %q missing title annotation on layer with digest %q", pkg.Name, layer.Digest)
		}
		layerHex := strings.TrimPrefix(layer.Digest, "sha256:")
		src := filepath.Join(blobDir, layerHex)
		dst, err := safeLayerDestinationPath(dstDir, title)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(dst), tempDirPerm); err != nil {
			return nil, fmt.Errorf("creating dir for layer %q: %w", title, err)
		}
		if err := copyFileContents(ctx, src, dst); err != nil {
			return nil, fmt.Errorf("staging layer %q for package %q: %w", title, pkg.Name, err)
		}
	}

	filter := BuildComponentFilter(pkg.OptionalComponents)
	// IsPartial: true because the bundle stores only the layers ingested at create time;
	// checksums.txt may reference blobs that were filtered out during bundle create.
	pkgLayout, err := layout.LoadFromDir(ctx, dstDir, layout.PackageLayoutOptions{Filter: filter, IsPartial: true})
	if err != nil {
		return nil, fmt.Errorf("loading package layout for %q: %w", pkg.Name, err)
	}
	return pkgLayout, nil
}

// SourcePackageLayoutLoader implements PackageLayoutLoader using the standard OCI/local pull
// path via NewPackageSource. It is the default loader when ZarfDeployer.Loader is nil.
type SourcePackageLayoutLoader struct {
	configOpts ConfigOptions
	bundleDir  string
}

var _ PackageLayoutLoader = (*SourcePackageLayoutLoader)(nil)

func (l *SourcePackageLayoutLoader) LoadPackageLayout(ctx context.Context, pkg *Package, dstDir string) (*layout.PackageLayout, error) {
	slog.Info("pulling package", "source", pkg.Source)
	source := NewPackageSource(pkg.Source, l.configOpts, l.bundleDir)
	filter := BuildComponentFilter(pkg.OptionalComponents)
	pkgLayout, err := source.PullFiltered(ctx, filter, dstDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load package %q from %s: %w", pkg.Name, pkg.Source, err)
	}
	return pkgLayout, nil
}

func copyFileContents(ctx context.Context, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, tmpFilePerm)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, &ctxReader{ctx: ctx, r: in}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return out.Sync()
}

// ctxReader wraps an io.Reader and checks ctx on every Read so large copies observe cancellation.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *ctxReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
