// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"log/slog"
)

// Remove removes a UDS bundle's packages from a Kubernetes cluster.
// It validates the bundle and delegates the bundle-level removal (DAG
// traversal, ordering, skip handling) to the Remover implementation.
// When opts.Packages is non-empty, only the specified packages are removed.
//
// IMPORTANT: The caller (pkg/cmd/bundle/remove.go) is responsible for validating
// the bundle path via util.ValidateBundlePath() and resolving it via
// util.ResolveBundlePath() before calling this function.
func Remove(ctx context.Context, opts RemoveOptions) (*RemoveResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	parser := NewHCLParser(opts.Config.Options.Architecture)

	// Use pre-parsed bundle if provided, otherwise parse from BundlePath
	b := opts.Bundle
	if b != nil {
		if err := b.Validate(); err != nil {
			return nil, fmt.Errorf("bundle validation failed: %w", err)
		}
	}
	if b == nil {
		slog.Debug("parsing bundle", "path", opts.BundlePath)
		var err error
		b, err = parser.ParseBundleFile(ctx, opts.BundlePath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse bundle: %w", err)
		}
		slog.Debug("bundle parsed", "name", b.Metadata.Name, "packages", len(b.Packages))

		// Validate only when freshly parsed (caller is responsible for pre-parsed bundles)
		if err := b.Validate(); err != nil {
			return nil, fmt.Errorf("bundle validation failed: %w", err)
		}
		slog.Debug("bundle validated")
	}

	remover := NewZarfRemover(opts.Out)
	return remover.RemoveBundle(ctx, b, opts.Packages, RemovePackageOptions{
		Config: opts.Config,
	})
}
