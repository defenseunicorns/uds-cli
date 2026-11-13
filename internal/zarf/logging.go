// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"log"
	"log/slog"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/zarf-dev/zarf/src/pkg/logger"
	"golang.org/x/sync/semaphore"
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

// Helm writes some warnings through global log/slog instead of IOStreams. A
// values-type conflict can include a password in the warning. Own those globals
// once per bundle; packages still run in parallel inside that scope.
var bundleLogLock = semaphore.NewWeighted(1)

type bundleLoggingScope struct{}

// silenceBundleLogs returns a restore function that the bundle must defer once.
// All bundles take the lock: a package hook can introduce sensitive values after
// orchestration starts. The context marker rejects nested hook deployments rather
// than letting them wait forever for the lock held by their parent bundle.
func silenceBundleLogs(ctx context.Context) (context.Context, func(), error) {
	if ctx.Value(bundleLoggingScope{}) != nil {
		return ctx, nil, ErrNestedBundleDeploy
	}
	// Unlike sync.Mutex, this wait respects cancellation while another bundle runs.
	if err := bundleLogLock.Acquire(ctx, 1); err != nil {
		return ctx, nil, err
	}
	previousLogger, previousWriter, previousFlags := slog.Default(), log.Writer(), log.Flags()
	// SetDefault also redirects standard log output. Explicit IOStreams loggers
	// remain independent, so UDS can keep reporting package progress.
	slog.SetDefault(slog.New(slog.DiscardHandler))
	restore := func() {
		// SetDefault changes the standard logger's writer and flags; restore both.
		slog.SetDefault(previousLogger)
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		bundleLogLock.Release(1)
	}
	return context.WithValue(ctx, bundleLoggingScope{}, true), restore, nil
}
