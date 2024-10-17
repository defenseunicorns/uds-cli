// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

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

// createSBOMArtifact creates sbom artifacts in the form of a tar archive
func createSBOMArtifact(SBOMArtifactPathMap map[string]string, bundleName string) error {
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

// moveExtractedSBOMs moves the extracted SBOM HTML and JSON files from src to dst
func moveExtractedSBOMs(bundleName, src, dst string) error {
	srcSBOMPath := filepath.Join(src, config.BundleSBOM)
	extractDirName := fmt.Sprintf("%s-%s", bundleName, config.BundleSBOM)
	sbomDir := filepath.Join(dst, extractDirName)

	// if sbomDir already exists, remove to make room for new sboms
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

// HandleSBOM handles the extraction and creation of bundle SBOMs after populating SBOMArtifactPathMap
func HandleSBOM(extractSBOM bool, SBOMArtifactPathMap map[string]string, bundleName, dstPath string) ([]string, error) {
	var warns []string

	if extractSBOM {
		if len(SBOMArtifactPathMap) == 0 {
			warns = append(warns, "Cannot extract, no SBOMs found in bundle")
			return warns, nil
		}
		currentDir, err := os.Getwd()
		if err != nil {
			return warns, err
		}
		err = moveExtractedSBOMs(bundleName, dstPath, currentDir)
		if err != nil {
			return warns, err
		}
	} else if len(SBOMArtifactPathMap) > 0 {
		err := createSBOMArtifact(SBOMArtifactPathMap, bundleName)
		if err != nil {
			return warns, err
		}
	} else {
		warns = append(warns, "No SBOMs found in bundle")
	}

	return warns, nil
}
