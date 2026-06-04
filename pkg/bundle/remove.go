// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/logger"
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

	s := logger.Bind(opts.Streams, opts.Config.Global.LogLevel)

	parser := NewHCLParser(opts.Config.Options.Architecture, s)

	// Use pre-parsed bundle if provided, otherwise parse from BundlePath
	b := opts.Bundle
	if b != nil {
		if err := b.Validate(); err != nil {
			return nil, fmt.Errorf("bundle validation failed: %w", err)
		}
	}
	if b == nil {
		s.Debug("parsing bundle", "path", opts.BundlePath)
		var err error
		b, err = parser.ParseBundleFile(ctx, opts.BundlePath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse bundle: %w", err)
		}
		s.Debug("bundle parsed", "name", b.Metadata.Name, "packages", len(b.Packages))

		// Validate only when freshly parsed (caller is responsible for pre-parsed bundles)
		if err := b.Validate(); err != nil {
			return nil, fmt.Errorf("bundle validation failed: %w", err)
		}
		s.Debug("bundle validated")
	}

	remover := NewZarfRemover(s)
	return remover.RemoveBundle(ctx, b, opts.Packages, RemovePackageOptions{
		Config: opts.Config,
	})
}
