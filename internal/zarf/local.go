// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/pkg/oci"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	zarfarchive "github.com/zarf-dev/zarf/src/pkg/archive"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// Compile-time check: localSource must implement PackageSource.
var _ PackageSource = &localSource{}

// extractTarZst extracts a compressed Zarf package into a destination directory.
func extractTarZst(ctx context.Context, streams iostreams.IOStreams, src, dst string) error {
	streams.Debug("extracting tar.zst archive", "src", src, "dst", dst)
	return zarfarchive.Decompress(ctx, src, dst, zarfarchive.DecompressOpts{OverwriteExisting: true})
}

// findOCILayoutRoot locates an OCI layout in the supported package directories.
func findOCILayoutRoot(root string) (string, error) {
	for _, dir := range []string{root, filepath.Join(root, "oci"), filepath.Join(root, "images")} {
		if isOCILayoutDir(dir) {
			return dir, nil
		}
	}
	return "", fmt.Errorf("no OCI image layout found in %q", root)
}

// isOCILayoutDir reports whether a directory has the required OCI layout structure.
func isOCILayoutDir(dir string) bool {
	if dir == "" {
		return false
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "oci-layout")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "index.json")); err != nil {
		return false
	}
	st, err := os.Stat(filepath.Join(dir, "blobs", "sha256"))
	return err == nil && st.IsDir()
}

// resolvedPath returns the absolute path to the local source, resolving
// relative paths against bundleDir.
func (s *localSource) resolvedPath() string {
	if filepath.IsAbs(s.path) {
		return s.path
	}
	return filepath.Join(s.bundleDir, s.path)
}

// PullFiltered loads a Zarf package from a local directory or archive,
// applying the filter. This enables Deploy from local sources.
func (s *localSource) PullFiltered(ctx context.Context, tmpDir string, loadOptions layout.PackageLayoutOptions) (*layout.PackageLayout, error) {
	if strings.TrimSpace(s.path) == "" {
		return nil, fmt.Errorf("local package source path is empty")
	}
	path := s.resolvedPath()
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", path, err)
	}

	if !st.IsDir() {
		if strings.HasSuffix(path, ".tar.zst") {
			if err := extractTarZst(ctx, s.streams, path, tmpDir); err != nil {
				return nil, fmt.Errorf("extracting local package %q: %w", s.path, err)
			}
			return layout.LoadFromDir(ctx, tmpDir, loadOptions)
		}
		return nil, fmt.Errorf("unsupported local package source %q", s.path)
	}

	if isZarfPackage(path) {
		// Copy the directory to tmpDir so that pkgLayout.Cleanup() (which calls
		// os.RemoveAll) does not destroy the user's source directory.
		copyDir := filepath.Join(tmpDir, "zarf-pkg")
		if err := helpers.CreatePathAndCopy(path, copyDir); err != nil {
			return nil, fmt.Errorf("copying local package to temp dir: %w", err)
		}
		return layout.LoadFromDir(ctx, copyDir, loadOptions)
	}

	return nil, fmt.Errorf("unsupported local package source %q: not a Zarf package directory or .tar.zst archive", path)
}

// VerifyAndIngestFiltered verifies a private staged copy of a local package and
// ingests that exact copy so the verified content cannot diverge from the
// content entering the bundle.
func (s *localSource) VerifyAndIngestFiltered(ctx context.Context, tmpDir string, loadOptions layout.PackageLayoutOptions, blobDir string) ([]ociManifest, error) {
	pkgLayout, err := s.PullFiltered(ctx, tmpDir, loadOptions)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := pkgLayout.Cleanup(); err != nil {
			s.streams.Warn("failed to remove verified package layout", "path", pkgLayout.DirPath(), "error", err)
		}
	}()

	stagedSource := &localSource{
		path:    pkgLayout.DirPath(),
		arch:    s.arch,
		tmpDir:  s.tmpDir,
		streams: s.streams,
	}
	return stagedSource.IngestFiltered(ctx, loadOptions.Filter, blobDir)
}

// IngestFiltered ingests a local Zarf package into the bundle's blob directory.
// Handles Zarf package directories, OCI layout directories, and .tar.zst archives.
// Applies component filtering after ingestion for Zarf packages.
func (s *localSource) IngestFiltered(ctx context.Context, filter filters.ComponentFilterStrategy, blobDir string) ([]ociManifest, error) {
	if strings.TrimSpace(s.path) == "" {
		return nil, fmt.Errorf("local package source path is empty")
	}
	path := s.resolvedPath()
	s.streams.Debug("ingesting local source", "path", path, "arch", s.arch)

	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", path, err)
	}

	// If it's a file, extract it first
	layoutRoot := path
	cleanup := func() {}
	if !st.IsDir() {
		tmp, err := os.MkdirTemp(s.tmpDir, "uds-local-oci-*")
		if err != nil {
			return nil, err
		}
		cleanup = func() {
			if err := os.RemoveAll(tmp); err != nil {
				s.streams.Warn("failed to remove temporary directory", "path", tmp, "error", err)
			}
		}

		if strings.HasSuffix(path, ".tar.zst") {
			if err := extractTarZst(ctx, s.streams, path, tmp); err != nil {
				cleanup()
				return nil, err
			}
			layoutRoot = tmp
		} else {
			cleanup()
			return nil, fmt.Errorf("unsupported local package source %q", s.path)
		}
	}
	defer cleanup()

	// Zarf package: convert to OCI layer format, then apply filter
	if isZarfPackage(layoutRoot) {
		manifests, err := ingestZarfPackage(ctx, s.streams, blobDir, layoutRoot, s.arch)
		if err != nil {
			return nil, err
		}
		for i, m := range manifests {
			filtered, err := filterIngestedManifest(ctx, s.streams, blobDir, m, filter)
			if err != nil {
				return nil, fmt.Errorf("filtering local zarf package: %w", err)
			}
			manifests[i] = filtered
		}
		return manifests, nil
	}

	// Standard OCI layout: copy blobs
	root, err := findOCILayoutRoot(layoutRoot)
	if err != nil {
		return nil, err
	}

	idxBytes, err := os.ReadFile(filepath.Join(root, "index.json"))
	if err != nil {
		return nil, err
	}
	var srcIndex ociIndex
	if err := json.Unmarshal(idxBytes, &srcIndex); err != nil {
		return nil, err
	}
	if len(srcIndex.Manifests) == 0 {
		return nil, fmt.Errorf("no manifests found in index.json")
	}

	srcBlobDir := filepath.Join(root, "blobs", "sha256")
	matched := filterOCIManifestsByArch(srcIndex.Manifests, s.arch)
	if len(matched) == 0 {
		return nil, fmt.Errorf("no manifests found matching architecture %q in %q", s.arch, s.path)
	}

	if err := udsoci.CopyRequiredBlobsFromLayout(blobDir, srcBlobDir, matched); err != nil {
		return nil, err
	}

	for i := range matched {
		if matched[i].MediaType == "" {
			matched[i].MediaType = specv1.MediaTypeImageManifest
		}
	}

	// Apply component filtering to any Zarf manifests in the OCI layout
	for i, m := range matched {
		filtered, err := filterIngestedManifest(ctx, s.streams, blobDir, m, filter)
		if err != nil {
			return nil, fmt.Errorf("filtering local OCI layout: %w", err)
		}
		matched[i] = filtered
	}

	return matched, nil
}

// ingestZarfPackage converts a traditional Zarf package directory to OCI layer format
// and ingests it into the bundle blob store. Each file in the Zarf package becomes
// an OCI layer with org.opencontainers.image.title annotation and file permissions.
func ingestZarfPackage(ctx context.Context, streams iostreams.IOStreams, blobDir, pkgRoot, arch string) ([]ociManifest, error) {
	streams.Debug("ingesting zarf package", "root", pkgRoot, "arch", arch)
	// Parse zarf.yaml for metadata if it exists
	zarfMeta := readZarfMetadata(filepath.Join(pkgRoot, "zarf.yaml"))

	// Walk the package directory and create layers for each file
	var layers []ociDescriptor

	err := filepath.WalkDir(pkgRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Check for context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.IsDir() {
			return nil
		}

		// Skip symlinks and irregular files
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}

		// Get relative path from package root
		relPath, err := filepath.Rel(pkgRoot, path)
		if err != nil {
			return err
		}

		// Use streaming for large files to avoid OOM
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		h := sha256.New()
		size, err := io.Copy(h, f)
		if err != nil {
			return err
		}
		hexStr := hex.EncodeToString(h.Sum(nil))
		d := udsoci.SHA256Digest(hexStr)

		// Write blob to bundle using streaming
		if err := udsoci.CopyBlobFileIfMissingAndVerify(blobDir, path, d); err != nil {
			return err
		}

		// Use forward slashes in annotations (OCI standard)
		title := filepath.ToSlash(relPath)

		// Store file permissions and size
		annotations := map[string]string{
			"org.opencontainers.image.title":      title,
			"org.defenseunicorns.zarf.file.mode":  fmt.Sprintf("%o", info.Mode().Perm()),
			"org.defenseunicorns.zarf.file.mtime": info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
		}

		layers = append(layers, ociDescriptor{
			MediaType:   mediaTypeZarfLayer,
			Digest:      d.String(),
			Size:        size,
			Annotations: annotations,
		})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking package directory: %w", err)
	}

	if len(layers) == 0 {
		return nil, fmt.Errorf("no files found in Zarf package")
	}

	// Create config blob with package metadata
	configData, err := json.Marshal(map[string]interface{}{
		"architecture": arch,
		"os":           oci.MultiOS,
		"config": map[string]interface{}{
			"Labels": map[string]string{
				"org.defenseunicorns.zarf.name":        zarfMeta.Metadata.Name,
				"org.defenseunicorns.zarf.version":     zarfMeta.Metadata.Version,
				"org.defenseunicorns.zarf.description": zarfMeta.Metadata.Description,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling config: %w", err)
	}
	configDigest, err := udsoci.WriteAndDigestBlob(blobDir, configData)
	if err != nil {
		return nil, fmt.Errorf("writing config blob: %w", err)
	}

	// Create the image manifest
	imageManifest := ociImageManifest{
		SchemaVersion: 2,
		MediaType:     specv1.MediaTypeImageManifest,
		Config: ociDescriptor{
			MediaType: specv1.MediaTypeImageConfig,
			Digest:    configDigest.String(),
			Size:      int64(len(configData)),
		},
		Layers: layers,
	}

	// Marshal and write manifest blob
	manifestData, err := json.Marshal(imageManifest)
	if err != nil {
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}

	manifestDigest, err := udsoci.WriteAndDigestBlob(blobDir, manifestData)
	if err != nil {
		return nil, fmt.Errorf("writing manifest blob: %w", err)
	}

	// Return the manifest descriptor (ref ADR-0015).
	return []ociManifest{{
		MediaType: specv1.MediaTypeImageManifest,
		Digest:    manifestDigest.String(),
		Size:      int64(len(manifestData)),
	}}, nil
}
