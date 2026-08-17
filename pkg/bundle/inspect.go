// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

type inspectTargetResolver func(context.Context, string, *InspectOptions) (udsoci.Target, error)

// Inspect reads metadata from a built local or OCI bundle. When a verification
// policy is provided, it verifies the bundle before parsing its metadata.
// Without a policy, it reports that the metadata is unverified.
func Inspect(ctx context.Context, opts InspectOptions) (*InspectResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	return inspect(ctx, opts, nil)
}

func inspect(ctx context.Context, opts InspectOptions, targetResolver inspectTargetResolver) (*InspectResult, error) {
	opts.Streams = logger.Bind(opts.Streams, opts.Config.Global.LogLevel)

	var resolver artifact.InspectTargetResolver
	if targetResolver != nil {
		resolver = func(ctx context.Context, source string, _ *artifact.InspectOptions) (udsoci.Target, error) {
			return targetResolver(ctx, source, &opts)
		}
	}

	inspectOpts := artifact.InspectOptions{
		Source:                 opts.Source,
		Config:                 toInternalConfig(opts.Config),
		Streams:                opts.Streams,
		CheckSignatureEvidence: opts.SkipSignatureVerification,
	}
	verificationConfigured := opts.Verification.configured()
	if !opts.SkipSignatureVerification && verificationConfigured {
		inspectOpts.VerifyBundle = func(ctx context.Context, index, evidence []byte) error {
			return verifySignature(ctx, index, evidence, opts.Verification, opts.Config.Options.TmpDir)
		}
	}
	internalResult, err := artifact.Inspect(ctx, inspectOpts, resolver)
	if err != nil {
		return nil, err
	}

	result, err := toInspectResult(ctx, internalResult.Bundle, opts.Streams)
	if err != nil {
		return nil, fmt.Errorf("inspecting bundle: %w", err)
	}
	result.ArtifactDigest = internalResult.ArtifactDigest
	result.ReconfiguredFrom = internalResult.ReconfiguredFrom
	if opts.SkipSignatureVerification {
		result.BundleSignature = &BundleSignatureSummary{Status: BundleSignatureStatusSkipped}
	} else if verificationConfigured {
		result.BundleSignature = &BundleSignatureSummary{Status: BundleSignatureStatusVerified}
	} else {
		result.BundleSignature = &BundleSignatureSummary{Status: BundleSignatureStatusUnverified}
		opts.Streams.Warn("bundle signature was not verified during inspection")
	}

	for i := range result.Packages {
		summary, ok := internalResult.PackageSignatures[result.Packages[i].Name]
		if !ok {
			return nil, fmt.Errorf("package %q was not found in inspected bundle", result.Packages[i].Name)
		}
		result.Packages[i].Signature = &PackageSignatureSummary{
			Signed:       packageSigningStatusString(summary.Signed),
			Verification: packageVerificationStatusString(summary.Verification),
		}
	}

	return result, nil
}

// toInspectResult converts a parsed bundle into its public inspection result.
// Packages are listed in DAG (deployment) order.
func toInspectResult(ctx context.Context, b *spec.UDSBundle, streams iostreams.IOStreams) (*InspectResult, error) {
	dag, err := bundleinternal.BuildDependencyGraph(ctx, streams, b)
	if err != nil {
		return nil, fmt.Errorf("building dependency graph: %w", err)
	}

	sorted, err := dag.TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("topological sort: %w", err)
	}

	result := &InspectResult{
		Name:        b.Metadata.Name,
		Description: b.Metadata.Description,
		Version:     b.Metadata.Version,
		Packages:    make([]PackageSummary, len(sorted)),
	}
	for i, pkg := range sorted {
		result.Packages[i] = toPackageSummary(pkg)
	}
	return result, nil
}

// toPackageSummary converts a package model to its inspect representation.
func toPackageSummary(pkg *spec.Package) PackageSummary {
	var depNames []string
	if len(pkg.DependsOn) > 0 {
		depNames = make([]string, len(pkg.DependsOn))
		for i, ref := range pkg.DependsOn {
			depNames[i] = ref.Name
		}
	}
	return PackageSummary{
		Name:        pkg.Name,
		Source:      pkg.Source,
		Namespace:   pkg.Namespace,
		DependsOn:   depNames,
		ValuesFiles: pkg.ValuesFiles,
	}
}

func packageSigningStatusString(status artifact.PackageSigningStatus) string {
	switch status {
	case artifact.PackageSigningStatusSigned:
		return PackageSigningStatusSigned
	case artifact.PackageSigningStatusUnsigned:
		return PackageSigningStatusUnsigned
	default:
		return PackageSigningStatusUnknown
	}
}

func packageVerificationStatusString(status artifact.PackageVerificationStatus) string {
	switch status {
	case artifact.PackageVerificationStatusVerified:
		return PackageVerificationStatusVerified
	case artifact.PackageVerificationStatusSkipped:
		return PackageVerificationStatusSkipped
	default:
		return PackageVerificationStatusUnknown
	}
}
