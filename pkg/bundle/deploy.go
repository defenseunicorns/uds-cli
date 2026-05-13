// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle implements UDS bundle deployment functionality.
//
// Below is an illustration of the data flow
//
//	bundle.uds.hcl ──▶ HCLParser ──▶ UDSBundle ──▶ Validate ──▶ DAG ──▶ Levels ──▶ Deploy(opts)
//	                                                              UDSBundleConfig (pre-resolved) ──┘
package bundle

import (
	"context"
	"fmt"
	"log/slog"
)

// Deploy deploys a UDS bundle to a Kubernetes cluster.
// It validates the bundle and delegates the bundle-level deployment (DAG
// traversal, ordering, parallelism, concurrency limits) to the Deployer
// implementation.
//
// When opts.Bundle is set, parsing and validation of the bundle file is skipped
// (the caller is responsible for having already validated the bundle).
//
// IMPORTANT: The caller (pkg/cmd/bundle/deploy.go) is responsible for validating
// the bundle path via util.ValidateBundlePath() and resolving it via
// util.ResolveBundlePath() before calling this function.
func Deploy(ctx context.Context, opts DeployOptions) (*DeployResult, error) {
	if err := ValidateConfig(opts.Config); err != nil {
		return nil, err
	}

	b := opts.Bundle
	if b == nil {
		slog.Debug("parsing bundle", "path", opts.BundlePath)
		parser := NewHCLParser(opts.Config.Options.Architecture)
		var err error
		b, err = parser.ParseBundleFile(ctx, opts.BundlePath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse bundle: %w", err)
		}
		slog.Debug("bundle parsed", "name", b.Metadata.Name, "packages", len(b.Packages))

		if err := b.Validate(); err != nil {
			return nil, fmt.Errorf("bundle validation failed: %w", err)
		}
		slog.Debug("bundle validated")
	}

	deployer := NewZarfDeployer(opts.Out)
	return deployer.DeployBundle(ctx, b, opts)
}
