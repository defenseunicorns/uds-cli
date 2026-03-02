// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package iostreams provides standard I/O stream abstractions for CLI commands.
package iostreams

import (
	"bytes"
	"io"
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
