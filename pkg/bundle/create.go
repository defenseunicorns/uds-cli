// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

// CreateOptions holds configuration for the top-level bundle create operation.
type CreateOptions struct {
	Config  *UDSBundleConfig
	Streams iostreams.IOStreams
	Signing SigningOptions
}

// CreateResult represents the output of a bundle create operation.
type CreateResult struct {
	BundleName string `json:"bundleName" yaml:"bundleName" text:"Bundle Name"`
	OutputPath string `json:"outputPath" yaml:"outputPath" text:"Output Path"`
}

// Create creates a UDS bundle tar.zst from the given bundle definition file.
// It parses and validates the bundle, ingests all packages, and writes the
// resulting archive next to the bundle file.
func Create(ctx context.Context, bundleFile string, opts CreateOptions) (*CreateResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if bundleFile == "" {
		return nil, fmt.Errorf("bundle file is required")
	}

	s := logger.Bind(opts.Streams, opts.Config.Options.LogLevel)

	s.Debug("parsing bundle file", "path", bundleFile)
	b, bundleHCL, err := parseAndMaterializeBundleFile(ctx, opts.Config.Options.Architecture, s, bundleFile)
	if err != nil {
		return nil, err
	}
	s.Debug("bundle parsed", "name", b.Metadata.Name, "packages", len(b.Packages))

	if err := artifact.ValidateBundleForCreate(b); err != nil {
		return nil, err
	}
	s.Debug("bundle validated")

	srcDir := filepath.Dir(bundleFile)
	var defaultsHCL []byte
	defaultsPath := filepath.Join(srcDir, bundleDefaultsFileName)
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
	if opts.Signing.Mode == "" || opts.Signing.Mode == SigningModeUnsigned {
		s.Warn("bundle is unsigned; its integrity and origin are not established")
	} else if err := Sign(ctx, SignOptions{Source: result.OutputPath, Signing: opts.Signing, Config: opts.Config, TmpDir: opts.Config.Options.TmpDir, Streams: s}); err != nil {
		if removeErr := os.Remove(result.OutputPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return nil, fmt.Errorf("signing created bundle and removing unsigned output: %w", errors.Join(err, removeErr))
		}
		return nil, fmt.Errorf("signing created bundle: %w", err)
	}
	return &CreateResult{BundleName: b.Metadata.Name, OutputPath: result.OutputPath}, nil
}
