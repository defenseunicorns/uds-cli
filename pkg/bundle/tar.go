// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mholt/archives"
	zarfarchive "github.com/zarf-dev/zarf/src/pkg/archive"
)

func writeTarZst(ctx context.Context, dst, srcDir string) (retErr error) {
	slog.Debug("writing tar.zst archive", "dst", dst)
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

func extractTarZst(ctx context.Context, src, dst string) error {
	slog.Debug("extracting tar.zst archive", "src", src, "dst", dst)
	return zarfarchive.Decompress(ctx, src, dst, zarfarchive.DecompressOpts{OverwriteExisting: true})
}
