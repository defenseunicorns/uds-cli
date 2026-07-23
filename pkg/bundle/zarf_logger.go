// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/zarf-dev/zarf/src/pkg/logger"
)

// newZarfLoggerContext attaches streams' logger to ctx so Zarf's packager output
// joins UDS CLI's own diagnostics. Callers logger.Bind first, so a logger is
// normally present; with none, ctx is returned unchanged (Zarf's From discards).
func newZarfLoggerContext(ctx context.Context, streams iostreams.IOStreams) context.Context {
	l := streams.Logger()
	if l == nil {
		return ctx
	}
	return logger.WithContext(ctx, l)
}
