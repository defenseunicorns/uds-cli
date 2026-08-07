// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"

	"github.com/defenseunicorns/uds-cli/internal/logger"
)

// Pull is a compatibility adapter over NewDefaultPuller().PullBundle.
func Pull(ctx context.Context, ociReference, targetDir string, opts PullOptions) (*PullResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	s := logger.Bind(opts.Streams, opts.Config.Global.LogLevel)
	s.Info("pulling bundle", "ref", ociReference)
	return NewDefaultPuller().PullBundle(ctx, ociReference, targetDir, opts)
}
