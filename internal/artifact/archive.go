// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/mholt/archives"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	zarfarchive "github.com/zarf-dev/zarf/src/pkg/archive"
)

const archiveFilePerm os.FileMode = 0o644

// WriteTarZst writes the regular files beneath srcDir to a tar.zst archive.
func WriteTarZst(ctx context.Context, streams iostreams.IOStreams, dst, srcDir string) (retErr error) {
	streams.Debug("writing tar.zst archive", "dst", dst)
	archivePerm := archiveFilePerm
	if st, err := os.Stat(dst); err == nil {
		if st.IsDir() {
			return OutputPathIsDirError{Path: dst}
		}
		archivePerm = st.Mode().Perm()
	} else if !os.IsNotExist(err) {
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
			return UnsupportedFileTypeError{Path: f.NameInArchive}
		}
		regular = append(regular, f)
	}
	slices.SortFunc(regular, func(a, b archives.FileInfo) int {
		return strings.Compare(a.NameInArchive, b.NameInArchive)
	})

	f, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	closed := false
	defer func() {
		if !closed {
			if fErr := f.Close(); fErr != nil && retErr == nil {
				retErr = fErr
			}
		}
		if retErr != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	ca := archives.CompressedArchive{
		Archival:    archives.Tar{},
		Compression: archives.Zstd{},
	}
	if err := ca.Archive(ctx, f, regular); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	closed = true
	if err := os.Chmod(tmpPath, archivePerm); err != nil {
		return fmt.Errorf("setting archive permissions: %w", err)
	}
	if runtime.GOOS == "windows" {
		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing existing archive: %w", err)
		}
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return err
	}
	return nil
}

// CountTarZstEntries returns the number of archive entries that extract to name.
func CountTarZstEntries(ctx context.Context, src, name string) (count int, retErr error) {
	f, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("opening archive: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("closing archive: %w", err)
		}
	}()

	zr, err := (archives.Zstd{}).OpenReader(f)
	if err != nil {
		return 0, fmt.Errorf("opening zstd archive: %w", err)
	}
	defer func() {
		if err := zr.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("closing zstd archive: %w", err)
		}
	}()

	tr := tar.NewReader(zr)
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			return count, nil
		}
		if err != nil {
			return 0, fmt.Errorf("reading tar archive: %w", err)
		}
		if path.Clean(hdr.Name) == name {
			count++
		}
	}
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
func CreateBundleArchive(ctx context.Context, streams iostreams.IOStreams, ociDir, targetDir string, idx ocispec.Index, arch string) (string, error) {
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
func bundleNameFromDefinitionLayer(ctx context.Context, streams iostreams.IOStreams, ociDir string, idx ocispec.Index, arch string) (string, error) {
	var cfgEntry *ocispec.Descriptor
	for i := range idx.Manifests {
		if idx.Manifests[i].ArtifactType == oci.MediaTypeBundleDefinition {
			cfgEntry = &idx.Manifests[i]
			break
		}
	}
	if cfgEntry == nil {
		return "", ErrManifestNotFound
	}

	store, err := oci.OpenReadOnlyStore(ociDir)
	if err != nil {
		return "", err
	}
	cfgBytes, err := oci.FetchBytes(ctx, store, *cfgEntry)
	if err != nil {
		return "", fmt.Errorf("%w %s: %w", ErrReadingConfigManifest, cfgEntry.Digest, err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(cfgBytes, &manifest); err != nil {
		return "", fmt.Errorf("%w %s: %w", ErrParsingConfigManifest, cfgEntry.Digest, err)
	}

	var hclDesc ocispec.Descriptor
	for _, l := range manifest.Layers {
		if l.MediaType == oci.MediaTypeBundleHCL {
			hclDesc = l
			break
		}
	}
	if hclDesc.Digest == "" {
		return "", ErrHCLLayerNotFound
	}

	hclBytes, err := oci.FetchBytes(ctx, store, hclDesc)
	if err != nil {
		return "", fmt.Errorf("%w %s: %w", ErrReadingHCLBlob, hclDesc.Digest, err)
	}

	b, err := bundleinternal.NewHCLParser(arch, streams).ParseBundleBytes(ctx, hclBytes)
	if err != nil {
		return "", fmt.Errorf("%w %s: %w", ErrParsingHCLBlob, hclDesc.Digest, err)
	}

	if arch == "" {
		arch = runtime.GOARCH
	}
	return bundleOutputName(b, arch), nil
}
