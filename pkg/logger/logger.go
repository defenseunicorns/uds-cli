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

// Bind ensures s has a logger writing to s.ErrOut at level, returning the updated
// copy. It is order-independent and safe to call repeatedly: a logger we created is
// re-leveled in place (last Bind wins), while a caller-supplied one (WithLogger) is
// honored and its level left untouched. level is validated upstream; unparseable
// values fall back to info.
func Bind(s iostreams.IOStreams, level string) iostreams.IOStreams {
	lvl, _ := ParseLevel(level)
	if s.Logger() != nil {
		if lv := s.LogLevel(); lv != nil {
			lv.Set(lvl)
		}
		return s
	}
	lv := new(slog.LevelVar)
	lv.Set(lvl)
	return s.WithLogger(New(s.ErrOut(), lv), lv)
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
