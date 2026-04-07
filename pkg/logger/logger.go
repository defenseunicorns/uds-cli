// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package logger

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	console "github.com/phsym/console-slog"
)

// New creates a new *slog.Logger that writes to w using console-slog for
// human-friendly colored output, unified with Zarf's log format.
// Passing a *slog.LevelVar as level allows the level to be changed at runtime.
func New(w io.Writer, level slog.Leveler) *slog.Logger {
	return slog.New(console.NewHandler(w, &console.HandlerOptions{Level: level}))
}

// ParseLevel converts a string to a slog.Level.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level %q, valid values are: debug, info, warn, error", s)
	}
}
