// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package logger

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	console "github.com/phsym/console-slog"
)

// New creates a new *slog.Logger that writes to w using console-slog for
// human-friendly colored output, unified with Zarf's log format.
// Passing a *slog.LevelVar as level allows the level to be changed at runtime.
//
// A nil w is treated as io.Discard so callers that pass an unset IOStreams.ErrOut
// get a no-op logger rather than a panic. Output is never redirected to a global
// stream (e.g. os.Stderr); no sink means no output.
func New(w io.Writer, level slog.Leveler) *slog.Logger {
	if w == nil {
		w = io.Discard
	}
	return slog.New(console.NewHandler(w, &console.HandlerOptions{Level: level}))
}

// Bind returns a copy of s whose leveled logging methods write to s.ErrOut at the
// given level. level is the string form ("debug"/"info"/...) already validated
// upstream by the config resolver; an unparseable value falls back to info.
// Library entrypoints call this once to attach the per-operation logger.
func Bind(s iostreams.IOStreams, level string) iostreams.IOStreams {
	lvl, _ := ParseLevel(level)
	return s.WithLogger(New(s.ErrOut, lvl))
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
