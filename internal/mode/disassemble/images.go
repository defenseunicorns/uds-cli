// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/archive"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

const sharedImageArchiveRelPath = "oci-layout.tar"

func localizeImages(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir string, component *v1alpha1.ZarfComponent) error {
	images := component.GetImages()
	if len(images) == 0 {
		component.Images = nil
		component.ImageArchives = nil
		return nil
	}
	archiveRel, err := ensureSharedImageArchive(ctx, pkgLayout, outputDir)
	if err != nil {
		return err
	}
	component.Images = nil
	component.ImageArchives = []v1alpha1.ImageArchive{{Path: archiveRel, Images: images}}
	return nil
}

func ensureSharedImageArchive(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir string) (string, error) {
	rel := filepath.ToSlash(sharedImageArchiveRelPath)
	dst := filepath.Join(outputDir, rel)
	if _, err := os.Stat(dst); err == nil {
		return rel, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), helpers.ReadWriteExecuteUser); err != nil {
		return "", fmt.Errorf("creating image archive directory: %w", err)
	}
	imagesRoot := pkgLayout.GetImageDirPath()
	entries, err := os.ReadDir(imagesRoot)
	if err != nil {
		return "", fmt.Errorf("reading image layout: %w", err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(imagesRoot, entry.Name()))
	}
	if err := archive.Compress(ctx, paths, dst, archive.CompressOpts{}); err != nil {
		return "", fmt.Errorf("creating image archive: %w", err)
	}
	return rel, nil
}
