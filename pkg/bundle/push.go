// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	"github.com/defenseunicorns/uds-cli/internal/logger"
)

// Push is a compatibility adapter that extracts the tarball and delegates to NewDefaultPusher().PushBundle.
// It preserves the current CLI tarball UX at the adapter layer.
func Push(ctx context.Context, bundleTarball, ociReference string, opts PushOptions) (*PushResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	s := logger.Bind(opts.Streams, opts.Config.Global.LogLevel)
	s.Info("pushing bundle", "tarball", bundleTarball, "ref", ociReference)
	signatureEntries, err := artifact.CountTarZstEntries(ctx, bundleTarball, BundleSignatureFileName)
	if err != nil {
		return nil, fmt.Errorf("checking bundle signature evidence: %w", err)
	}
	if signatureEntries > 1 {
		return nil, fmt.Errorf("expected exactly one bundle signature evidence entry, found %d", signatureEntries)
	}

	tmp, err := os.MkdirTemp(opts.Config.Options.TmpDir, "uds-bundle-push-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() {
		if rerr := os.RemoveAll(tmp); rerr != nil {
			s.Warn("failed to remove temporary directory", "path", tmp, "error", rerr)
		}
	}()

	s.Debug("extracting bundle", "source", bundleTarball, "output", tmp)
	if err := artifact.ExtractTarZst(ctx, s, bundleTarball, tmp); err != nil {
		return nil, fmt.Errorf("extracting bundle: %w", err)
	}

	return NewDefaultPusher().PushBundle(ctx, tmp, ociReference, opts)
}
