// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"errors"
	"fmt"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	internalzarf "github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

// RemoveOptions contains options for removing an entire bundle.
type RemoveOptions struct {
	Config                    *UDSBundleConfig
	Packages                  []string
	Verification              VerificationPolicy
	SkipSignatureVerification bool
	// Force bypasses the removal-safety check for a selected package subset. It
	// can remove packages still required by remaining bundle packages, leaving
	// deployed dependents broken.
	Force   bool
	Streams iostreams.IOStreams
}

type removePackageOptions struct {
	Config               *UDSBundleConfig
	DeployedPackageNames map[string]string
	Force                bool
	SafetyChecked        bool
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
		return nil, fmt.Errorf("source is required: %w", ErrSourceRequired)
	}
	if source.BundlePath == "" && source.Bundle == nil {
		return nil, fmt.Errorf("source must provide BundlePath or Bundle: %w", ErrBundleInputRequired)
	}

	s := logger.Bind(opts.Streams, opts.Config.Options.LogLevel)

	if udsoci.IsOCIReference(source.BundlePath) || artifact.IsTarZst(source.BundlePath) {
		metadata, err := artifact.OpenMetadataSource(ctx, source.BundlePath, toInternalConfig(opts.Config))
		if err != nil {
			return nil, fmt.Errorf("%w %q: opening artifact metadata: %w", ErrRemoveBundle, source.BundlePath, err)
		}
		prepared, err := prepareArtifactRemoveSource(ctx, metadata, opts, s)
		if err != nil {
			return nil, fmt.Errorf("%w %q: preparing artifact: %w", ErrRemoveBundle, source.BundlePath, err)
		}
		source = prepared
		defer func() { _ = prepared.Close() }()
	}

	b := source.Bundle
	if b == nil {
		s.Debug("parsing bundle", "path", source.BundlePath)
		var err error
		b, err = parseBundleFile(ctx, opts.Config.Options.Architecture, s, source.BundlePath)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to parse bundle: %w", ErrRemoveBundle, err)
		}
	}
	s.Debug("bundle parsed", "name", b.Metadata.Name, "packages", len(b.Packages))

	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("%w: bundle validation failed: %w", ErrRemoveBundle, err)
	}
	s.Debug("bundle validated")
	if err := bundleinternal.ValidatePackageNames(opts.Packages, b.Packages); err != nil {
		return nil, fmt.Errorf("%w %q: validating package selection: %w", ErrRemoveBundle, b.Metadata.Name, err)
	}
	deployedPackageNames, err := artifactPackageNames(source, b, opts.Packages)
	if err != nil {
		return nil, err
	}
	if !opts.Force {
		if err := validateRemovalSafety(ctx, s, b, opts.Packages); err != nil {
			return nil, fmt.Errorf("%w: unable to remove safely: %w", ErrRemoveBundle, err)
		}
	}

	remover := newZarfRemover(s)
	result, err := remover.removeBundle(ctx, b, opts.Packages, removePackageOptions{
		Config:               opts.Config,
		DeployedPackageNames: deployedPackageNames,
		Force:                opts.Force,
		SafetyChecked:        !opts.Force,
	})
	if err != nil {
		return result, fmt.Errorf("%w %q: %w", ErrRemoveBundle, b.Metadata.Name, err)
	}
	if result == nil {
		return nil, fmt.Errorf("%w: remover returned no result", ErrRemoveBundle)
	}
	removed, skipped := countRemovalResults(result.Packages)
	s.Info("bundle removal complete", "name", result.BundleName, "removed", removed, "skipped", skipped)
	return result, nil
}

func prepareArtifactRemoveSource(ctx context.Context, source *artifact.MetadataSource, opts RemoveOptions, streams iostreams.IOStreams) (*DeploySource, error) {
	if !opts.SkipSignatureVerification {
		policy := opts.Verification
		if !policy.configured() && opts.Config.SignatureVerification != nil {
			policy = *opts.Config.SignatureVerification
		}
		if err := policy.Validate(); err != nil {
			return nil, err
		}
		evidence, err := source.FetchSignatureEvidence(ctx)
		if err != nil {
			if errors.Is(err, udsoci.ErrBundleSignatureNotFound) {
				return nil, fmt.Errorf("%w: %w", ErrBundleNotSigned, err)
			}
			return nil, fmt.Errorf("fetching bundle signature evidence: %w", err)
		}
		if policy.configured() {
			if err := verifySignature(ctx, source.IndexBytes, evidence, policy, opts.Config.Options.TmpDir); err != nil {
				return nil, fmt.Errorf("verifying bundle signature: %w", err)
			}
		}
	}

	metadata, err := artifact.ReadBundleDefinition(ctx, source, streams)
	if err != nil {
		return nil, err
	}
	zarfNames, err := artifact.ReadZarfPackageNames(ctx, source, metadata.Bundle, opts.Packages...)
	if err != nil {
		return nil, err
	}
	return &DeploySource{Bundle: metadata.Bundle, packageZarfNames: zarfNames}, nil
}

func artifactPackageNames(source *DeploySource, b *spec.UDSBundle, packages []string) (map[string]string, error) {
	if source == nil || source.packageZarfNames == nil {
		return nil, nil
	}
	selected := make(map[string]struct{}, len(packages))
	for _, name := range packages {
		selected[name] = struct{}{}
	}
	names := make(map[string]string, len(b.Packages))
	for i := range b.Packages {
		pkg := &b.Packages[i]
		if len(selected) > 0 {
			if _, ok := selected[pkg.Name]; !ok {
				continue
			}
		}
		zarfName := source.packageZarfNames[pkg.Name]
		if zarfName == "" {
			return nil, fmt.Errorf("%w %q: embedded Zarf package name is required", ErrRemoveBundle, pkg.Name)
		}
		names[pkg.Name] = zarfName
	}
	return names, nil
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

	result, err := r.remover.RemoveBundle(ctx, b, packages, internalzarf.RemovePackageOptions{
		Config:               toZarfConfig(opts.Config),
		DeployedPackageNames: opts.DeployedPackageNames,
		Force:                opts.Force,
	})
	if result == nil {
		return nil, err
	}

	packageResults := make([]RemovePackageResult, len(result.Packages))
	for i, pkg := range result.Packages {
		packageResults[i] = RemovePackageResult{Name: pkg.Name, Status: RemovePackageStatus(pkg.Status)}
	}
	return &RemoveResult{BundleName: result.BundleName, Packages: packageResults}, err
}
