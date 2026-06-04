// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package iostreams provides standard I/O stream abstractions for CLI commands.
package iostreams

import (
	"bytes"
	"io"
	"log/slog"
	"os"
)

// IOStreams provides the standard names for iostreams.
type IOStreams struct {
	// In is the stdin reader.
	In io.Reader
	// Out is the stdout writer.
	Out io.Writer
	// ErrOut is the stderr writer.
	ErrOut io.Writer
	// log is an optional structured logger; nil means leveled methods are no-ops.
	log *slog.Logger
}

// NewIOStreams returns an IOStreams pointing to os.Stdin, os.Stdout, and os.Stderr.
func NewIOStreams() IOStreams {
	return IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
}

// NewTestIOStreams returns a valid IOStreams for testing with bytes.Buffer.
// It returns the IOStreams and separate buffers for In, Out, and ErrOut for inspection.
func NewTestIOStreams() (IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}

	return IOStreams{
		In:     in,
		Out:    out,
		ErrOut: errOut,
	}, in, out, errOut
}

// WithLogger returns a copy of s whose Debug/Info/Warn/Error methods delegate to l.
// The caller is responsible for configuring l to write to the appropriate destination
// (typically ErrOut) via its handler.
func (s IOStreams) WithLogger(l *slog.Logger) IOStreams {
	s.log = l
	return s
}

// Debug writes a debug-level diagnostic via the configured logger.
// It is a no-op when no logger has been set (see WithLogger).
func (s IOStreams) Debug(msg string, args ...any) {
	if s.log != nil {
		s.log.Debug(msg, args...)
	}
}

// Info writes an info-level diagnostic via the configured logger.
// It is a no-op when no logger has been set (see WithLogger).
func (s IOStreams) Info(msg string, args ...any) {
	if s.log != nil {
		s.log.Info(msg, args...)
	}
}

// Warn writes a warning-level diagnostic via the configured logger.
// It is a no-op when no logger has been set (see WithLogger).
func (s IOStreams) Warn(msg string, args ...any) {
	if s.log != nil {
		s.log.Warn(msg, args...)
	}
}

// Error writes an error-level diagnostic via the configured logger.
// It is a no-op when no logger has been set (see WithLogger).
func (s IOStreams) Error(msg string, args ...any) {
	if s.log != nil {
		s.log.Error(msg, args...)
	}
}
