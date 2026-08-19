// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/uds-cli/internal/logger"
	internalzarf "github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

// RemoveOptions contains options for removing an entire bundle.
type RemoveOptions struct {
	Config   *UDSBundleConfig
	Packages []string
	// Force bypasses the removal-safety check for a selected package subset. It
	// can remove packages still required by remaining bundle packages, leaving
	// deployed dependents broken.
	Force   bool
	Streams iostreams.IOStreams
}

type removePackageOptions struct {
	Config        *UDSBundleConfig
	Force         bool
	SafetyChecked bool
}

// RemoveResult represents the output of a bundle remove operation.
type RemoveResult struct {
	BundleName string                `json:"bundleName" yaml:"bundleName" text:"Bundle Name"`
	Packages   []RemovePackageResult `json:"packages" yaml:"packages" text:"Packages"`
}

// RemovePackageResult represents the outcome for one package in a bundle removal.
type RemovePackageResult struct {
	Name   string              `json:"name" yaml:"name" text:"Name"`
	Status RemovePackageStatus `json:"status" yaml:"status" text:"Status"`
}

// RemovePackageStatus describes whether a package was removed or was already absent.
type RemovePackageStatus string

const (
	RemovePackageStatusRemoved RemovePackageStatus = "removed"
	RemovePackageStatusSkipped RemovePackageStatus = "skipped"
)

// Remove validates and removes a UDS bundle from a Kubernetes cluster.
// When opts.Packages is non-empty, only the specified packages are removed.
func Remove(ctx context.Context, source *DeploySource, opts RemoveOptions) (*RemoveResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if source == nil {
		return nil, fmt.Errorf("source is required")
	}
	if source.BundlePath == "" && source.Bundle == nil {
		return nil, fmt.Errorf("source must provide BundlePath or Bundle")
	}

	s := logger.Bind(opts.Streams, opts.Config.Options.LogLevel)

	b := source.Bundle
	if b == nil {
		s.Debug("parsing bundle", "path", source.BundlePath)
		var err error
		b, err = parseBundleFile(ctx, opts.Config.Options.Architecture, s, source.BundlePath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse bundle: %w", err)
		}
	}
	s.Debug("bundle parsed", "name", b.Metadata.Name, "packages", len(b.Packages))

	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("bundle validation failed: %w", err)
	}
	s.Debug("bundle validated")
	if !opts.Force {
		if err := validateRemovalSafety(ctx, s, b, opts.Packages); err != nil {
			return nil, err
		}
	}

	remover := newZarfRemover(s)
	result, err := remover.removeBundle(ctx, b, opts.Packages, removePackageOptions{
		Config:        opts.Config,
		Force:         opts.Force,
		SafetyChecked: !opts.Force,
	})
	if err != nil {
		return result, err
	}
	removed, skipped := countRemovalResults(result.Packages)
	s.Info("bundle removal complete", "name", result.BundleName, "removed", removed, "skipped", skipped)
	return result, nil
}

func countRemovalResults(packages []RemovePackageResult) (removed, skipped int) {
	for _, pkg := range packages {
		switch pkg.Status {
		case RemovePackageStatusRemoved:
			removed++
		case RemovePackageStatusSkipped:
			skipped++
		}
	}
	return removed, skipped
}

type zarfRemover struct {
	remover *internalzarf.ZarfRemover
	streams iostreams.IOStreams
}

func newZarfRemover(streams iostreams.IOStreams) *zarfRemover {
	return &zarfRemover{remover: internalzarf.NewZarfRemover(streams), streams: streams}
}

func (r *zarfRemover) removeBundle(ctx context.Context, b *spec.UDSBundle, packages []string, opts removePackageOptions) (*RemoveResult, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	if !opts.Force && !opts.SafetyChecked && b != nil {
		if err := validateRemovalSafety(ctx, r.streams, b, packages); err != nil {
			return nil, err
		}
	}
	result, err := r.remover.RemoveBundle(ctx, b, packages, internalzarf.RemovePackageOptions{Config: toZarfConfig(opts.Config), Force: opts.Force})
	if result == nil {
		return nil, err
	}
	packageResults := make([]RemovePackageResult, len(result.Packages))
	for i, pkg := range result.Packages {
		packageResults[i] = RemovePackageResult{Name: pkg.Name, Status: RemovePackageStatus(pkg.Status)}
	}
	return &RemoveResult{BundleName: result.BundleName, Packages: packageResults}, err
}
