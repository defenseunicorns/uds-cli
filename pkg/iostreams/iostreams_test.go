// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package iostreams

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewIOStreams(t *testing.T) {
	streams := NewIOStreams()

	assert.Equal(t, os.Stdin, streams.In)
	assert.Equal(t, os.Stdout, streams.Out)
	assert.Equal(t, os.Stderr, streams.ErrOut)
}

func TestNewTestIOStreams(t *testing.T) {
	streams, in, out, errOut := NewTestIOStreams()

	assert.NotNil(t, streams.In)
	assert.NotNil(t, streams.Out)
	assert.NotNil(t, streams.ErrOut)

	testData := "test data"

	in.WriteString(testData)
	assert.Equal(t, testData, in.String())

	out.WriteString(testData)
	assert.Equal(t, testData, out.String())

	errOut.WriteString(testData)
	assert.Equal(t, testData, errOut.String())

	streams.In.(*bytes.Buffer).WriteString("input")
	assert.Equal(t, testData+"input", in.String())
}
