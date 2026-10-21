// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package modify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/helpers/v2"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/pkg/archive"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

const sharedImageArchiveRelPath = "oci-layout.tar"

func localizeImages(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir, tmpRoot string, component *api.Component) error {
	images := component.GetImages()
	if len(images) == 0 {
		component.Images = nil
		component.ImageArchives = nil
		return nil
	}
	archiveRel, err := ensureSharedImageArchive(ctx, pkgLayout, outputDir, tmpRoot)
	if err != nil {
		return err
	}
	component.Images = nil
	component.ImageArchives = []api.ImageArchive{{Path: archiveRel, Images: images}}
	return nil
}

func ensureSharedImageArchive(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir, tmpRoot string) (string, error) {
	rel := filepath.ToSlash(sharedImageArchiveRelPath)
	dst := filepath.Join(outputDir, rel)
	if _, err := os.Stat(dst); err == nil {
		return rel, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), helpers.ReadWriteExecuteUser); err != nil {
		return "", fmt.Errorf("creating image archive directory: %w", err)
	}
	imagesRoot := pkgLayout.GetImageDirPath()
	indexPath := filepath.Join(imagesRoot, ocispec.ImageIndexFile)
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		return "", fmt.Errorf("reading image index: %w", err)
	}
	var index ocispec.Index
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		return "", fmt.Errorf("parsing image index: %w", err)
	}
	for i := range index.Manifests {
		annotations := index.Manifests[i].Annotations
		if annotations[ocispec.AnnotationRefName] == "" && annotations[ocispec.AnnotationBaseImageName] != "" {
			annotations[ocispec.AnnotationRefName] = annotations[ocispec.AnnotationBaseImageName]
		}
	}
	indexBytes, err = json.Marshal(index)
	if err != nil {
		return "", fmt.Errorf("marshaling image archive index: %w", err)
	}
	// Zarf's archive importer needs ref.name for Crane-built packages that
	// carry only base.name. Stage the index without altering the loaded layout.
	indexPath = filepath.Join(tmpRoot, ocispec.ImageIndexFile)
	if err := os.WriteFile(indexPath, indexBytes, helpers.ReadWriteUser); err != nil {
		return "", fmt.Errorf("writing image archive index: %w", err)
	}
	entries, err := os.ReadDir(imagesRoot)
	if err != nil {
		return "", fmt.Errorf("reading image layout: %w", err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(imagesRoot, entry.Name())
		if entry.Name() == ocispec.ImageIndexFile {
			path = indexPath
		}
		paths = append(paths, path)
	}
	if err := archive.Compress(ctx, paths, dst, archive.CompressOpts{}); err != nil {
		return "", fmt.Errorf("creating image archive: %w", err)
	}
	return rel, nil
}
