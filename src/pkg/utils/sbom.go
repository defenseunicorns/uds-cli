// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package utils provides utility fns for UDS-CLI
package utils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/mholt/archiver/v4"
)

// CreateSBOMArtifact creates sbom artifacts in the form of a tar archive
func CreateSBOMArtifact(SBOMArtifactPathMap map[string]string, bundleName string) error {
	out, err := os.Create(fmt.Sprintf("%s-%s", bundleName, config.BundleSBOMTar))
	if err != nil {
		return err
	}
	defer out.Close()
	files, err := archiver.FilesFromDisk(nil, SBOMArtifactPathMap)
	if err != nil {
		return err
	}
	format := archiver.Tar{}
	err = format.Archive(context.TODO(), out, files)
	if err != nil {
		return err
	}
	return nil
}

// MoveExtractedSBOMs moves the extracted SBOM HTML and JSON files from src to dst
func MoveExtractedSBOMs(bundleName, src, dst string) error {
	srcSBOMPath := filepath.Join(src, config.BundleSBOM)
	extractDirName := fmt.Sprintf("%s-%s", bundleName, config.BundleSBOM)
	sbomDir := filepath.Join(dst, extractDirName)

	// is sbomDir already exists, remove to make room for new sboms
	if _, err := os.Stat(sbomDir); err == nil {
		err = os.RemoveAll(sbomDir)
		if err != nil {
			return err
		}
	}

	err := os.Rename(srcSBOMPath, sbomDir)
	if err != nil {
		return err
	}

	return nil
}

// SBOMExtractor is the extraction fn for extracting HTML and JSON files from an sboms.tar archive
func SBOMExtractor(dst string, SBOMArtifactPathMap map[string]string) func(_ context.Context, f archiver.File) error {
	extractor := func(_ context.Context, f archiver.File) error {
		open, err := f.Open()
		if err != nil {
			return err
		}
		size := f.Size() - 1
		if size > 0 {
			buffer := make([]byte, size)
			_, err = open.Read(buffer)
			if err != nil {
				return err
			}
			err = open.Close()
			if err != nil {
				return err
			}
			path := filepath.Join(dst, config.BundleSBOM, f.NameInArchive)
			// todo: handle collisions? especially for zarf-component SBOM files?
			err = os.WriteFile(path, buffer, 0600)
			if err != nil {
				return err
			}
			// map files for bundle-level sboms.tar
			SBOMArtifactPathMap[path] = f.NameInArchive
		}
		return nil
	}
	return extractor
}
