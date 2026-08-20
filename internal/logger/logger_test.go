// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package logger

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, slog.LevelWarn)
	require.NotNil(t, l)

	l.Info("should be suppressed")
	assert.Empty(t, buf.String(), "info message should be suppressed at warn level")

	l.Warn("visible warning")
	assert.Contains(t, buf.String(), "visible warning")
}

func TestNew_NilWriterDoesNotPanic(t *testing.T) {
	l := New(nil, slog.LevelInfo)
	require.NotNil(t, l)
	// Writing through a nil-backed logger must be a safe no-op, not a panic.
	l.Info("goes nowhere")
}

func TestBind_RoutesToErrOutAtLevel(t *testing.T) {
	var buf bytes.Buffer
	s := iostreams.New(nil, nil, &buf)
	s = Bind(s, "warn")

	s.Info("suppressed-at-warn")
	s.Warn("visible-warn")

	out := buf.String()
	assert.NotContains(t, out, "suppressed-at-warn")
	assert.Contains(t, out, "visible-warn")
}

func TestBind_InvalidLevelFallsBackToInfo(t *testing.T) {
	// Level is validated upstream; Bind itself is lenient and defaults to info.
	var buf bytes.Buffer
	s := Bind(iostreams.New(nil, nil, &buf), "not-a-level")

	s.Debug("debug-suppressed")
	s.Info("info-visible")

	out := buf.String()
	assert.NotContains(t, out, "debug-suppressed")
	assert.Contains(t, out, "info-visible")
}

func TestBind_HonorsPreBoundLogger(t *testing.T) {
	var caller, console bytes.Buffer
	callerLogger := slog.New(slog.NewJSONHandler(&caller, nil))

	s := iostreams.New(nil, nil, &console).WithLogger(callerLogger, nil)
	s = Bind(s, "info")

	require.Same(t, callerLogger, s.Logger(), "Bind must not replace a caller-supplied logger")
	require.Nil(t, s.LogLevel(), "a caller-supplied logger exposes no LevelVar; its level is not ours to change")

	s.Info("routed-to-caller")
	assert.Contains(t, caller.String(), "routed-to-caller")
	assert.Empty(t, console.String(), "no console logger may be created when one is pre-bound")
}

func TestBind_LeavesCallerLevelUntouched(t *testing.T) {
	// A caller-pinned error level must not be widened by a later verbose Bind.
	var caller bytes.Buffer
	callerLogger := slog.New(slog.NewTextHandler(&caller, &slog.HandlerOptions{Level: slog.LevelError}))

	s := iostreams.New(nil, nil, nil).WithLogger(callerLogger, nil)
	s = Bind(s, "debug")

	s.Debug("should-stay-suppressed")
	assert.Empty(t, caller.String())
}

func TestBind_RelevelsOwnLogger(t *testing.T) {
	// Last Bind wins for a logger we created, independent of ordering.
	var buf bytes.Buffer
	s := Bind(iostreams.New(nil, nil, &buf), "info")
	s.Debug("suppressed-at-info")

	s = Bind(s, "debug")
	require.NotNil(t, s.LogLevel())
	s.Debug("visible-at-debug")

	out := buf.String()
	assert.NotContains(t, out, "suppressed-at-info")
	assert.Contains(t, out, "visible-at-debug")
	assert.Same(t, s.Logger(), Bind(s, "warn").Logger(), "re-leveling reuses the same logger")
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input     string
		wantLevel slog.Level
		wantErr   bool
	}{
		{input: "debug", wantLevel: slog.LevelDebug},
		{input: "DEBUG", wantLevel: slog.LevelDebug},
		{input: "info", wantLevel: slog.LevelInfo},
		{input: "INFO", wantLevel: slog.LevelInfo},
		{input: "warn", wantLevel: slog.LevelWarn},
		{input: "warning", wantLevel: slog.LevelWarn},
		{input: "WARNING", wantLevel: slog.LevelWarn},
		{input: "error", wantLevel: slog.LevelError},
		{input: "ERROR", wantLevel: slog.LevelError},
		{input: "invalid", wantErr: true},
		{input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level, err := ParseLevel(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidLevel)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantLevel, level)
		})
	}
}
