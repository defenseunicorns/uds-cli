// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"io"

	"github.com/zarf-dev/zarf/src/pkg/logger"
)

// newZarfLoggerContext returns ctx with a Zarf logger attached, configured to
// write to out at logLevel. Used by both ZarfDeployer and ZarfRemover so the
// log format stays consistent across deploy/remove and color handling lives
// in a single place. logLevel is already validated by the config resolver;
// ParseLevel will not fail here.
func newZarfLoggerContext(ctx context.Context, out io.Writer, logLevel string) context.Context {
	level, _ := logger.ParseLevel(logLevel)

	cfg := logger.Config{
		Level:       level,
		Format:      logger.FormatConsole,
		Destination: logger.Destination(out),
		Color:       true,
	}
	l, err := logger.New(cfg)
	if err != nil {
		l = logger.Default()
	}
	return logger.WithContext(ctx, l)
}
