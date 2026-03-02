// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package logger

import (
	"bytes"
	"log/slog"
	"testing"

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
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantLevel, level)
		})
	}
}
