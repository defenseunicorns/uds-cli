// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/filesystem"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// LoadOptions carries options for package layout loading.
type LoadOptions struct {
	// Streams carries diagnostics for the loader.
	Streams iostreams.IOStreams
	// IsPartial reports whether the loaded package may omit checksum-referenced layers.
	IsPartial bool
}

// PackageLayoutLoader loads a package into a deployable Zarf layout.
type PackageLayoutLoader interface {
	LoadPackageLayout(context.Context, *spec.Package, string, LoadOptions) (*layout.PackageLayout, bool, error)
}

// ExtractedArtifactPackageLayoutLoader reads package OCI blobs from an extracted bundle artifact.
type ExtractedArtifactPackageLayoutLoader struct {
	OCIDir           string
	PackageManifests map[string]ocispec.Descriptor
}

// SourcePackageLayoutLoader loads packages from their declared local or OCI sources.
type SourcePackageLayoutLoader struct {
	configOpts bundleinternal.ConfigOptions
	bundleDir  string
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

var _ PackageLayoutLoader = (*ExtractedArtifactPackageLayoutLoader)(nil)

// PackageStagingRoot returns the artifact workspace so package layers can be
// staged alongside their OCI blobs using rooted hard links.
func (l *ExtractedArtifactPackageLayoutLoader) PackageStagingRoot(_ context.Context) string {
	if l.OCIDir == "" {
		return ""
	}
	return filepath.Dir(filepath.Clean(l.OCIDir))
}

// LoadPackageLayout stages indexed OCI layers into dstDir.
func (l *ExtractedArtifactPackageLayoutLoader) LoadPackageLayout(ctx context.Context, pkg *spec.Package, dstDir string, opts LoadOptions) (*layout.PackageLayout, bool, error) {
	s := opts.Streams
	s.Debug("loading package layout", "name", pkg.Name, "dir", dstDir)
	key := pkg.Name
	descriptor, ok := l.PackageManifests[key]
	if !ok {
		keys := make([]string, 0, len(l.PackageManifests))
		for k := range l.PackageManifests {
			keys = append(keys, k)
		}
		return nil, false, fmt.Errorf("package %q (source %q) not found in bundle artifact index; available: %v: %w", pkg.Name, pkg.Source, keys, ErrPackageNotFoundInArtifact)
	}

	cleanOCIDir, err := filepath.Abs(filepath.Clean(l.OCIDir))
	if err != nil {
		return nil, false, fmt.Errorf("resolving artifact OCI directory: %w", err)
	}
	blobDir := filepath.Join(cleanOCIDir, "blobs", "sha256")
	store, err := udsoci.OpenReadOnlyStore(cleanOCIDir)
	if err != nil {
		return nil, false, fmt.Errorf("package %q: %w: %w", pkg.Name, ErrOpenOCILayout, err)
	}
	manifestData, err := udsoci.FetchBytes(ctx, store, descriptor)
	if err != nil {
		return nil, false, fmt.Errorf("package %q: %w: %w", pkg.Name, ErrReadPackageManifest, err)
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, false, fmt.Errorf("package %q: %w: %w", pkg.Name, ErrReadPackageManifest, err)
	}

	cleanDstDir, err := filepath.Abs(filepath.Clean(dstDir))
	if err != nil {
		return nil, false, fmt.Errorf("%w %q for package %q: %w", ErrResolveDestinationDirectory, dstDir, pkg.Name, err)
	}
	dstRoot, err := os.OpenRoot(cleanDstDir)
	if err != nil {
		return nil, false, fmt.Errorf("opening package destination directory: %w", err)
	}
	defer func() { _ = dstRoot.Close() }()
	workspaceDir := filepath.Dir(cleanOCIDir)
	workspaceRoot, err := os.OpenRoot(workspaceDir)
	if err != nil {
		return nil, false, fmt.Errorf("opening artifact workspace directory: %w", err)
	}
	defer func() { _ = workspaceRoot.Close() }()
	for _, layer := range manifest.Layers {
		title := layer.Annotations[ocispec.AnnotationTitle]
		if title == "" {
			return nil, false, fmt.Errorf("manifest for package %q missing title annotation on layer with digest %q: %w", pkg.Name, layer.Digest, ErrMissingLayerTitle)
		}
		src := filepath.Join(blobDir, layer.Digest.Hex())
		relSrc, err := filepath.Rel(workspaceDir, src)
		if err != nil {
			return nil, false, fmt.Errorf("resolving layer %q relative to artifact workspace: %w", title, err)
		}
		dst, err := safeLayerDestinationPath(cleanDstDir, cleanDstDir, title)
		if err != nil {
			return nil, false, err
		}
		relDst, err := filepath.Rel(cleanDstDir, dst)
		if err != nil {
			return nil, false, fmt.Errorf("resolving layer %q relative to package destination: %w", title, err)
		}
		if err := dstRoot.MkdirAll(filepath.Dir(relDst), filesystem.PrivateDirectoryMode); err != nil {
			return nil, false, fmt.Errorf("layer %q: %w: %w", title, ErrCreateLayerDirectory, err)
		}
		workspaceDst, err := filepath.Rel(workspaceDir, dst)
		stageRoot := dstRoot
		stageDst := relDst
		// Hard links require both paths beneath one os.Root. External library
		// destinations remain supported through the rooted-copy fallback.
		canLink := err == nil && workspaceDst != ".." && !strings.HasPrefix(workspaceDst, ".."+string(os.PathSeparator))
		if canLink {
			stageRoot = workspaceRoot
			stageDst = workspaceDst
		}
		if err := stageArtifactPackageLayer(ctx, workspaceRoot, relSrc, stageRoot, stageDst, title, canLink); err != nil {
			return nil, false, fmt.Errorf("layer %q for package %q: %w: %w", title, pkg.Name, ErrStagePackageLayer, err)
		}
	}

	filter := BuildComponentFilter(pkg.OptionalComponents)
	// OCI-blob-staged packages always use IsPartial: true — the bundle stores only the
	// layers ingested at create time; checksums.txt may reference filtered-out blobs.
	pkgLayout, err := layout.LoadFromDir(ctx, dstDir, artifactPackageLayoutOptions(filter, true))
	if err != nil {
		return nil, false, fmt.Errorf("package %q: %w: %w", pkg.Name, ErrLoadPackage, err)
	}
	return pkgLayout, true, nil
}

// stageArtifactPackageLayer links regular immutable blobs inside one workspace
// and otherwise copies through rooted source and destination access.
func stageArtifactPackageLayer(ctx context.Context, workspaceRoot *os.Root, src string, dstRoot *os.Root, dst, title string, canLink bool) error {
	if canLink && filepath.ToSlash(title) != "images/index.json" && !rootPathContainsSymlink(workspaceRoot, src) {
		if err := workspaceRoot.Link(src, dst); err == nil {
			return nil
		}
	}
	return copyFileContentsBetweenRoots(ctx, workspaceRoot, src, dstRoot, dst)
}

// rootPathContainsSymlink reports whether name includes a symbolic-link
// component beneath root. Symlinked blobs are copied so their targets are not
// relocated into the package staging directory.
func rootPathContainsSymlink(root *os.Root, name string) bool {
	path := "."
	for _, component := range strings.Split(filepath.Clean(name), string(os.PathSeparator)) {
		if component == "." {
			continue
		}
		path = filepath.Join(path, component)
		info, err := root.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}
	return false
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
func (l *SourcePackageLayoutLoader) LoadPackageLayout(ctx context.Context, pkg *spec.Package, dstDir string, opts LoadOptions) (*layout.PackageLayout, bool, error) {
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
		return nil, false, fmt.Errorf("failed to load package %q from %s: %w: %w", pkg.Name, pkg.Source, ErrLoadPackage, err)
	}
	advisoryVerifyPackage(ctx, pkgLayout, pkg, l.configOpts.TmpDir, s)
	return pkgLayout, opts.IsPartial, nil
}

// copyFileContentsBetweenRoots copies src atomically between rooted
// filesystems while observing context cancellation.
func copyFileContentsBetweenRoots(ctx context.Context, srcRoot *os.Root, src string, dstRoot *os.Root, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	in, err := srcRoot.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, tmp, err := createRootTempFile(dstRoot, dst)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, &ctxReader{ctx: ctx, r: in})
	closeErr := out.Close()

	if copyErr != nil || ctx.Err() != nil {
		_ = dstRoot.Remove(tmp)
		if copyErr != nil {
			return copyErr
		}
		return ctx.Err()
	}
	if closeErr != nil {
		_ = dstRoot.Remove(tmp)
		return closeErr
	}
	return dstRoot.Rename(tmp, dst)
}

func createRootTempFile(root *os.Root, dst string) (*os.File, string, error) {
	var suffix [8]byte
	for range 10 {
		if _, err := rand.Read(suffix[:]); err != nil {
			return nil, "", err
		}
		tmp := filepath.Join(filepath.Dir(dst), fmt.Sprintf(".uds-copy-%x", suffix))
		file, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filesystem.PrivateFileMode)
		if os.IsExist(err) {
			continue
		}
		return file, tmp, err
	}
	return nil, "", fmt.Errorf("creating unique temporary file for %q", dst)
}

// safeLayerDestinationPath resolves a layer title without allowing directory escape.
func safeLayerDestinationPath(cleanDstDir, dstDir, title string) (string, error) {
	dst := filepath.Join(dstDir, filepath.FromSlash(title))

	cleanDst, err := filepath.Abs(filepath.Clean(dst))
	if err != nil {
		return "", fmt.Errorf("%w %q: %w", ErrResolveLayerTitle, title, err)
	}
	rel, err := filepath.Rel(cleanDstDir, cleanDst)
	if err != nil {
		return "", fmt.Errorf("%w %q: %w", ErrCheckLayerTitle, title, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", LayerPathEscapeError{Title: title}
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
