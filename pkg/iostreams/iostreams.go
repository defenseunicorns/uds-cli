// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package iostreams provides standard I/O stream abstractions for CLI commands.
package iostreams

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"sync"
)

// Output synchronization
//
// Parallel deploys have N goroutines writing to a single terminal, so
// synchronization is required. We use byte-level locking (lockedWriter): every
// Write call is serialized, giving live progress at the cost of possible
// mid-line interleaving. Zarf emits one line per Write, so interleaving is
// rare in practice.
//
// Sync lives here so every consumer — leveled logger methods and the raw
// Out/ErrOut accessors — goes through the same lock automatically. Always
// construct IOStreams via New, NewIOStreams, or NewTestIOStreams; the zero value
// is nil-safe (Out/ErrOut return io.Discard, In returns nil) but unprotected.

// lockedWriter wraps an io.Writer with a mutex so concurrent goroutines
// (parallel package deploys within a level) do not corrupt each other's
// writes. See the "Output synchronization" doc above for the rationale.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (lw *lockedWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.w.Write(p)
}

// synchronize wraps w in a lockedWriter. It is idempotent: if w is already a
// *lockedWriter it is returned unchanged. nil is passed through unchanged.
func synchronize(w io.Writer) io.Writer {
	if w == nil {
		return nil
	}
	if _, ok := w.(*lockedWriter); ok {
		return w
	}
	return &lockedWriter{w: w}
}

// IOStreams provides the standard names for iostreams.
// Use New, NewIOStreams, or NewTestIOStreams to construct instances and access
// streams via the In(), Out(), and ErrOut() accessors.
type IOStreams struct {
	in     io.Reader
	out    io.Writer // *lockedWriter via any public constructor, or nil in the zero value
	errOut io.Writer // *lockedWriter via any public constructor, or nil in the zero value
	// log is an optional structured logger; nil means leveled methods are no-ops.
	log *slog.Logger
}

// New builds an IOStreams over caller-supplied streams, synchronizing Out/ErrOut.
// nil out/errOut are stored as nil; Out() and ErrOut() return io.Discard for nil fields
// so direct writes are always safe. If the same writer is passed for both out
// and errOut, the two locks are independent; callers combining sinks must
// pre-synchronize the underlying writer.
func New(in io.Reader, out, errOut io.Writer) IOStreams {
	return IOStreams{
		in:     in,
		out:    synchronize(out),
		errOut: synchronize(errOut),
	}
}

// NewIOStreams returns an IOStreams pointing to os.Stdin, os.Stdout, and os.Stderr.
func NewIOStreams() IOStreams {
	return IOStreams{
		in:     os.Stdin,
		out:    synchronize(os.Stdout),
		errOut: synchronize(os.Stderr),
	}
}

// NewTestIOStreams returns a valid IOStreams for testing with bytes.Buffer.
// It returns the IOStreams and separate buffers for In, Out, and ErrOut for inspection.
func NewTestIOStreams() (IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}

	return IOStreams{
		in:     in,
		out:    synchronize(out),
		errOut: synchronize(errOut),
	}, in, out, errOut
}

// In returns the stdin reader.
func (s IOStreams) In() io.Reader { return s.in }

// Out returns the stdout writer. Returns io.Discard if no writer was configured.
func (s IOStreams) Out() io.Writer {
	if s.out == nil {
		return io.Discard
	}
	return s.out
}

// ErrOut returns the stderr writer. Returns io.Discard if no writer was configured.
func (s IOStreams) ErrOut() io.Writer {
	if s.errOut == nil {
		return io.Discard
	}
	return s.errOut
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
