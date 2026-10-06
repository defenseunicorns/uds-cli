// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package modify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/zarf-dev/zarf/src/api"
)

const (
	disassembleVersionSuffix = "-disassembled"
	disassemblyFormatVersion = "v1alpha1"
	disassemblyMetadataDir   = ".uds"
	disassemblyMetadataFile  = "disassembly.json"
)

type disassemblyMetadata struct {
	FormatVersion string `json:"formatVersion"`
	Architecture  string `json:"architecture"`
	Flavor        string `json:"flavor"`
}

func normalizeMetadata(metadata *api.PackageMetadata) {
	if metadata.Version == "" {
		metadata.Version = strings.TrimPrefix(disassembleVersionSuffix, "-")
	} else if !strings.HasSuffix(metadata.Version, disassembleVersionSuffix) {
		metadata.Version += disassembleVersionSuffix
	}
}

func writeDisassemblyMetadata(sourceDir, architecture, flavor string) error {
	if strings.TrimSpace(architecture) == "" {
		return errors.New("package build architecture is required")
	}
	metadata := disassemblyMetadata{
		FormatVersion: disassemblyFormatVersion,
		Architecture:  architecture,
		Flavor:        flavor,
	}
	contents, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling disassembly metadata: %w", err)
	}
	contents = append(contents, '\n')

	dir := filepath.Join(sourceDir, disassemblyMetadataDir)
	if err := os.MkdirAll(dir, helpers.ReadWriteExecuteUser); err != nil {
		return fmt.Errorf("creating disassembly metadata directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, disassemblyMetadataFile), contents, helpers.ReadWriteUser); err != nil {
		return fmt.Errorf("writing disassembly metadata: %w", err)
	}
	return nil
}

func readDisassemblyMetadata(sourceDir string) (disassemblyMetadata, error) {
	path := filepath.Join(sourceDir, disassemblyMetadataDir, disassemblyMetadataFile)
	contents, err := os.ReadFile(path)
	if err != nil {
		return disassemblyMetadata{}, fmt.Errorf("reading disassembly metadata: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var metadata disassemblyMetadata
	if err := decoder.Decode(&metadata); err != nil {
		return disassemblyMetadata{}, fmt.Errorf("decoding disassembly metadata: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return disassemblyMetadata{}, errors.New("decoding disassembly metadata: unexpected trailing content")
	}
	if metadata.FormatVersion != disassemblyFormatVersion {
		return disassemblyMetadata{}, fmt.Errorf("unsupported disassembly metadata format version %q", metadata.FormatVersion)
	}
	if strings.TrimSpace(metadata.Architecture) == "" {
		return disassemblyMetadata{}, errors.New("disassembly metadata architecture is required")
	}
	return metadata, nil
}

func validateOutputDir(outputDir string) error {
	info, err := os.Stat(outputDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking output directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("output path is not a directory: %s", outputDir)
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return fmt.Errorf("reading output directory: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("output directory must be empty: %s", outputDir)
	}
	return nil
}
