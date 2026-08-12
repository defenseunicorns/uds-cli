// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/mholt/archives"
	zarfarchive "github.com/zarf-dev/zarf/src/pkg/archive"
)

// WriteTarZst writes the regular files beneath srcDir to a tar.zst archive.
func WriteTarZst(ctx context.Context, streams iostreams.IOStreams, dst, srcDir string) (retErr error) {
	streams.Debug("writing tar.zst archive", "dst", dst)
	if st, err := os.Stat(dst); err == nil {
		if st.IsDir() {
			return fmt.Errorf("output path %q is a directory", dst)
		}
		if err := os.Remove(dst); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Trailing separator tells FilesFromDisk to enumerate the directory contents
	// without adding the directory entry itself to the archive.
	files, err := archives.FilesFromDisk(ctx, &archives.FromDiskOptions{ClearAttributes: true}, map[string]string{
		srcDir + string(filepath.Separator): "",
	})
	if err != nil {
		return err
	}

	// Filter to regular files; error on unexpected types (e.g. symlinks).
	regular := files[:0:0]
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if !f.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type %q in bundle staging dir", f.NameInArchive)
		}
		regular = append(regular, f)
	}
	slices.SortFunc(regular, func(a, b archives.FileInfo) int {
		return strings.Compare(a.NameInArchive, b.NameInArchive)
	})

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if fErr := f.Close(); fErr != nil && retErr == nil {
			retErr = fErr
		}
		if retErr != nil {
			_ = os.Remove(dst)
		}
	}()

	ca := archives.CompressedArchive{
		Archival:    archives.Tar{},
		Compression: archives.Zstd{},
	}
	return ca.Archive(ctx, f, regular)
}

// ExtractTarZst extracts a tar.zst archive into dst.
func ExtractTarZst(ctx context.Context, streams iostreams.IOStreams, src, dst string) error {
	streams.Debug("extracting tar.zst archive", "src", src, "dst", dst)
	return zarfarchive.Decompress(ctx, src, dst, zarfarchive.DecompressOpts{OverwriteExisting: true})
}

// IsTarZst reports whether s has a .tar.zst extension.
func IsTarZst(s string) bool {
	return strings.HasSuffix(s, ".tar.zst")
}

// CreateBundleArchive writes a pulled OCI bundle layout to targetDir.
func CreateBundleArchive(ctx context.Context, streams iostreams.IOStreams, ociDir, targetDir string, idx oci.OciIndex, arch string) (string, error) {
	name, err := bundleNameFromDefinitionLayer(ctx, streams, ociDir, idx, arch)
	if err != nil {
		return "", err
	}
	workspace := filepath.Dir(ociDir)
	outPath := filepath.Join(targetDir, name)
	if err := WriteTarZst(ctx, streams, outPath, workspace); err != nil {
		return "", err
	}
	return outPath, nil
}

// bundleNameFromDefinitionLayer reads the bundle HCL from the bundle definition manifest
// in the OCI index and derives the output filename using bundleOutputName.
// It assumes isBundleIndex has already been called to confirm the index is valid.
func bundleNameFromDefinitionLayer(ctx context.Context, streams iostreams.IOStreams, ociDir string, idx oci.OciIndex, arch string) (string, error) {
	var cfgEntry *oci.OciManifest
	for i := range idx.Manifests {
		if idx.Manifests[i].ArtifactType == oci.MediaTypeBundleDefinition {
			cfgEntry = &idx.Manifests[i]
			break
		}
	}
	if cfgEntry == nil {
		return "", fmt.Errorf("bundle definition manifest not found in index")
	}

	cfgHex := strings.TrimPrefix(cfgEntry.Digest, "sha256:")
	cfgBytes, err := os.ReadFile(filepath.Join(ociDir, "blobs", "sha256", cfgHex))
	if err != nil {
		return "", fmt.Errorf("reading config manifest blob: %w", err)
	}

	var manifest oci.OciImageManifest
	if err := json.Unmarshal(cfgBytes, &manifest); err != nil {
		return "", fmt.Errorf("parsing config manifest: %w", err)
	}

	var hclDigest string
	for _, l := range manifest.Layers {
		if l.MediaType == oci.MediaTypeBundleHCL {
			hclDigest = l.Digest
			break
		}
	}
	if hclDigest == "" {
		return "", fmt.Errorf("bundle HCL layer not found in config manifest")
	}

	hclHex := strings.TrimPrefix(hclDigest, "sha256:")
	hclBytes, err := os.ReadFile(filepath.Join(ociDir, "blobs", "sha256", hclHex))
	if err != nil {
		return "", fmt.Errorf("reading HCL blob: %w", err)
	}

	b, err := bundleinternal.NewHCLParser(arch, streams).ParseBundleBytes(ctx, hclBytes)
	if err != nil {
		return "", fmt.Errorf("parsing bundle HCL: %w", err)
	}

	if arch == "" {
		arch = runtime.GOARCH
	}
	return bundleOutputName(b, arch), nil
}
