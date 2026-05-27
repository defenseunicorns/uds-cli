// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// ExtractedBundle holds the results of extracting a .tar.zst bundle artifact
// into a caller-owned destination directory. Dir is shaped like a bundle source
// directory so that existing config-resolution code (ParseBundleFile,
// loadBundleDefaults) works without modification.
//
// Directory lifecycle is the caller's responsibility. ExtractArtifact writes
// into Dir but does not create or remove it.
type ExtractedBundle struct {
	// Dir is the root dir that contains bundle.uds.hcl (and optionally
	// defaults.uds.hcl and values/<pkg>/<idx>.yaml) at the top level.
	Dir string

	// OCIDir is the path to the extracted OCI image layout (Dir+"/oci").
	OCIDir string

	// BundleDefPath is the absolute path to the materialized bundle definition file (bundle.uds.hcl).
	BundleDefPath string

	// PackageDigests maps each package's source identifier to its manifest digest
	// ("sha256:..."). Keyed by the org.opencontainers.image.ref.name annotation
	// written during bundle create. For OCI sources this is the trimmed ref
	// (no oci:// prefix); for local paths it is the package name.
	PackageDigests map[string]string
}

// ValuesFilesByPackage walks the values/<pkg>/*.yaml directory tree
// materialized by ExtractArtifact and returns a map of package name to
// ordered list of absolute file paths. Files are sorted numerically by the
// index in their filename stem (0.yaml < 1.yaml < ... < 10.yaml).
func (e *ExtractedBundle) ValuesFilesByPackage() (map[string][]string, error) {
	result := make(map[string][]string)
	valuesRoot := filepath.Join(e.Dir, "values")

	pkgDirs, err := os.ReadDir(valuesRoot)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading values directory: %w", err)
	}

	for _, pkgEntry := range pkgDirs {
		if !pkgEntry.IsDir() {
			continue
		}
		pkgName := pkgEntry.Name()
		pkgDir := filepath.Join(valuesRoot, pkgName)

		fileEntries, err := os.ReadDir(pkgDir)
		if err != nil {
			return nil, fmt.Errorf("reading values for package %s: %w", pkgName, err)
		}

		type indexedPath struct {
			idx  int
			path string
		}
		var indexed []indexedPath

		for _, fe := range fileEntries {
			if fe.IsDir() || !strings.HasSuffix(fe.Name(), ".yaml") {
				continue
			}
			stem := strings.TrimSuffix(fe.Name(), ".yaml")
			n, err := strconv.Atoi(stem)
			if err != nil {
				// Non-numeric filename — skip (not created by bundle create).
				continue
			}
			indexed = append(indexed, indexedPath{idx: n, path: filepath.Join(pkgDir, fe.Name())})
		}

		sort.Slice(indexed, func(i, j int) bool { return indexed[i].idx < indexed[j].idx })

		paths := make([]string, len(indexed))
		for i, ip := range indexed {
			paths[i] = ip.path
		}
		if len(paths) > 0 {
			result[pkgName] = paths
		}
	}

	return result, nil
}

// ExtractArtifact extracts a .tar.zst bundle artifact into dstDir, verifies
// OCI layout digests, and materializes bundle.uds.hcl, defaults.uds.hcl, and
// values files at the top of dstDir so existing source-dir code paths work
// without modification. Returns an ExtractedBundle with package digest info.
//
// dstDir must already exist; the caller owns its lifecycle (creation and
// cleanup). On failure, extracted files may remain in dstDir. The caller is
// responsible for cleanup in all cases.
func ExtractArtifact(ctx context.Context, tarPath, dstDir string) (*ExtractedBundle, error) {
	slog.Debug("extracting bundle artifact", "tar", tarPath, "dst", dstDir)

	if err := extractTarZst(ctx, tarPath, dstDir); err != nil {
		return nil, fmt.Errorf("extracting bundle artifact: %w", err)
	}

	ociDir := filepath.Join(dstDir, "oci")
	blobDir := filepath.Join(ociDir, "blobs", "sha256")

	if err := verifyOCILayoutDigests(ociDir); err != nil {
		return nil, fmt.Errorf("artifact digest verification failed: %w", err)
	}

	idxBytes, err := os.ReadFile(filepath.Join(ociDir, "index.json"))
	if err != nil {
		return nil, fmt.Errorf("reading index.json: %w", err)
	}
	var idx ociIndex
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		return nil, fmt.Errorf("parsing index.json: %w", err)
	}
	if !isBundleIndex(idx) {
		return nil, fmt.Errorf("%s does not appear to be a UDS bundle: no bundle definition manifest found", tarPath)
	}

	defEntry, defIdx, err := findBundleDefinitionEntry(idx)
	if err != nil {
		return nil, fmt.Errorf("locating bundle definition: %w", err)
	}

	defHex := strings.TrimPrefix(defEntry.Digest, "sha256:")
	defBytes, err := os.ReadFile(filepath.Join(blobDir, defHex))
	if err != nil {
		return nil, fmt.Errorf("reading bundle definition manifest: %w", err)
	}
	var bundleDef ociImageManifest
	if err := json.Unmarshal(defBytes, &bundleDef); err != nil {
		return nil, fmt.Errorf("parsing bundle definition manifest: %w", err)
	}

	bundleDefPath, err := materializeBundleSrcFiles(bundleDef, blobDir, dstDir)
	if err != nil {
		return nil, err
	}

	packageDigests, err := buildPackageDigests(idx, defIdx)
	if err != nil {
		return nil, err
	}

	slog.Debug("bundle artifact extracted", "dir", dstDir, "packages", len(packageDigests))
	return &ExtractedBundle{
		Dir:            dstDir,
		OCIDir:         ociDir,
		BundleDefPath:  bundleDefPath,
		PackageDigests: packageDigests,
	}, nil
}

// materializeBundleSrcFiles writes HCL and values file layers from the bundle
// definition manifest to disk at their annotated title paths under dstDir.
// Returns the absolute path to the materialized bundle.uds.hcl file.
func materializeBundleSrcFiles(bundleDef ociImageManifest, blobDir, dstDir string) (string, error) {
	var bundleDefPath string
	for _, layer := range bundleDef.Layers {
		title := layer.Annotations[ocispec.AnnotationTitle]
		if title == "" {
			continue
		}
		if layer.MediaType != MediaTypeBundleHCL && layer.MediaType != MediaTypeBundleValuesYAML {
			continue
		}

		hex := strings.TrimPrefix(layer.Digest, "sha256:")
		data, err := os.ReadFile(filepath.Join(blobDir, hex))
		if err != nil {
			return "", fmt.Errorf("reading layer blob %s (%s): %w", title, layer.Digest, err)
		}

		dst, err := safeLayerDestinationPath(dstDir, title)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(dst), tempDirPerm); err != nil {
			return "", fmt.Errorf("creating directory for %s: %w", title, err)
		}
		if err := os.WriteFile(dst, data, tmpFilePerm); err != nil {
			return "", fmt.Errorf("writing %s: %w", title, err)
		}
		if layer.MediaType == MediaTypeBundleHCL && title == BundleFileName {
			bundleDefPath = dst
		}
	}
	if bundleDefPath == "" {
		return "", fmt.Errorf("bundle artifact contains no bundle definition layer")
	}
	return bundleDefPath, nil
}

// safeLayerDestinationPath returns the filesystem destination for an OCI layer
// title under dstDir and rejects titles that would escape dstDir.
func safeLayerDestinationPath(dstDir, title string) (string, error) {
	dst := filepath.Join(dstDir, filepath.FromSlash(title))

	cleanDstDir, err := filepath.Abs(filepath.Clean(dstDir))
	if err != nil {
		return "", fmt.Errorf("resolving destination directory: %w", err)
	}
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

// buildPackageDigests returns a map of source identifier to manifest digest for
// all non-bundle-def entries in the index. The source identifier is taken from
// the org.opencontainers.image.ref.name annotation set by the create operation.
func buildPackageDigests(idx ociIndex, defIdx int) (map[string]string, error) {
	digests := make(map[string]string)
	for i, m := range idx.Manifests {
		if i == defIdx {
			continue
		}
		refName := m.Annotations["org.opencontainers.image.ref.name"]
		if refName == "" {
			return nil, fmt.Errorf("package manifest at index %d has no org.opencontainers.image.ref.name annotation", i)
		}
		if existing, ok := digests[refName]; ok && existing != m.Digest {
			return nil, fmt.Errorf("duplicate org.opencontainers.image.ref.name %q with conflicting digests (%s vs %s)", refName, existing, m.Digest)
		}
		digests[refName] = m.Digest
	}
	return digests, nil
}
