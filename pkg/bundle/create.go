// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/logger"
)

// Create creates a UDS bundle tar.zst from the given bundle definition file.
// It parses and validates the bundle, ingests all packages, and writes the
// resulting archive next to the bundle file.
func Create(ctx context.Context, opts CreateOptions) (*CreateResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	s := logger.Bind(opts.Streams, opts.Config.Global.LogLevel)

	s.Debug("parsing bundle file", "path", opts.BundleFile)
	parser := NewHCLParser(opts.Config.Options.Architecture, s)
	b, bundleHCL, err := parser.parseAndMaterializeBundleFile(ctx, opts.BundleFile)
	if err != nil {
		return nil, err
	}
	s.Debug("bundle parsed", "name", b.Metadata.Name, "packages", len(b.Packages))

	if err := artifact.ValidateBundleForCreate(b); err != nil {
		return nil, err
	}
	s.Debug("bundle validated")

	srcDir := filepath.Dir(opts.BundleFile)
	var defaultsHCL []byte
	defaultsPath := filepath.Join(srcDir, BundleDefaultsFileName)
	if _, err := os.Stat(defaultsPath); err == nil {
		defaultsHCL, err = bundleinternal.MaterializeDefaultsFile(defaultsPath)
		if err != nil {
			return nil, fmt.Errorf("materializing defaults HCL: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("accessing defaults HCL: %w", err)
	}
	result, err := artifact.Create(ctx, artifact.CreateOptions{
		Config:      toInternalConfig(opts.Config),
		Bundle:      b,
		BundleHCL:   bundleHCL,
		DefaultsHCL: defaultsHCL,
		BundleDir:   srcDir,
		Streams:     s,
	})
	if err != nil {
		return nil, err
	}
	return &CreateResult{BundleName: b.Metadata.Name, OutputPath: result.OutputPath}, nil
}
