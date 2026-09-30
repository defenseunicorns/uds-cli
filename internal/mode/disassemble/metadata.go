// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/zarf-dev/zarf/src/api/v1alpha1"
)

const disassembleVersionSuffix = "-disassembled"

func normalizeMetadata(metadata *v1alpha1.ZarfMetadata) {
	metadata.AggregateChecksum = ""
	if metadata.Version == "" {
		metadata.Version = strings.TrimPrefix(disassembleVersionSuffix, "-")
	} else if !strings.HasSuffix(metadata.Version, disassembleVersionSuffix) {
		metadata.Version += disassembleVersionSuffix
	}
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
