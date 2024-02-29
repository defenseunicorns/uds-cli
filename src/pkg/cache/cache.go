// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cache provides a primitive cache mechanism for bundle layers
package cache

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/src/config"
)

func expandTilde(cachePath string) string {
	if cachePath[:2] == "~/" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Error in cache dir: %v\n", err)
			return ""
		}
		return filepath.Join(homeDir, cachePath[2:])
	}
	return cachePath
}

// Add adds a file to the cache
func Add(filePathToAdd string) error {
	// ensure cache dir exists
	cacheDir := config.CommonOptions.CachePath
	if err := os.MkdirAll(filepath.Join(cacheDir, config.UDSCacheLayers), 0o755); err != nil {
		return err
	}

	// if file already in cache, return
	filename := strings.Split(filePathToAdd, config.BlobsDir)[1]
	if Exists(filename) {
		return nil
	}

	srcFile, err := os.Open(filePathToAdd)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(filepath.Join(cacheDir, config.UDSCacheLayers, filename))
	if err != nil {
		return err
	}
	defer dstFile.Close()
	_, err = io.Copy(dstFile, srcFile)
	return err
}

// Exists checks if a layer exists in the cache
func Exists(layerDigest string) bool {
	cacheDir := config.CommonOptions.CachePath
	layerCachePath := filepath.Join(expandTilde(cacheDir), config.UDSCacheLayers, layerDigest)
	_, err := os.Stat(layerCachePath)
	return !os.IsNotExist(err)
}

// Use copies a layer from the cache to the dst dir
func Use(layerDigest, dstDir string) error {
	cacheDir := config.CommonOptions.CachePath
	layerCachePath := filepath.Join(expandTilde(cacheDir), config.UDSCacheLayers, layerDigest)
	srcFile, err := os.Open(layerCachePath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// ensure blobs/sha256 dir has been created
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	dstFile, err := os.Create(filepath.Join(dstDir, layerDigest))
	if err != nil {
		return err
	}
	defer dstFile.Close()
	_, err = io.Copy(dstFile, srcFile)
	return err
}
