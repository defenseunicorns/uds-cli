// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package iostreams

import (
	"bytes"
	"os"
	"testing"
)

func TestNewIOStreams(t *testing.T) {
	streams := NewIOStreams()

	if streams.In != os.Stdin {
		t.Errorf("NewIOStreams().In = %v, want %v", streams.In, os.Stdin)
	}
	if streams.Out != os.Stdout {
		t.Errorf("NewIOStreams().Out = %v, want %v", streams.Out, os.Stdout)
	}
	if streams.ErrOut != os.Stderr {
		t.Errorf("NewIOStreams().ErrOut = %v, want %v", streams.ErrOut, os.Stderr)
	}
}

func TestNewTestIOStreams(t *testing.T) {
	streams, in, out, errOut := NewTestIOStreams()

	if streams.In == nil {
		t.Error("NewTestIOStreams().In should not be nil")
	}
	if streams.Out == nil {
		t.Error("NewTestIOStreams().Out should not be nil")
	}
	if streams.ErrOut == nil {
		t.Error("NewTestIOStreams().ErrOut should not be nil")
	}

	testData := "test data"

	in.WriteString(testData)
	if in.String() != testData {
		t.Errorf("in buffer = %q, want %q", in.String(), testData)
	}

	out.WriteString(testData)
	if out.String() != testData {
		t.Errorf("out buffer = %q, want %q", out.String(), testData)
	}

	errOut.WriteString(testData)
	if errOut.String() != testData {
		t.Errorf("errOut buffer = %q, want %q", errOut.String(), testData)
	}

	streams.In.(*bytes.Buffer).WriteString("input")
	if in.String() != testData+"input" {
		t.Errorf("streams.In not connected to in buffer")
	}
}
