// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package iostreams

import (
	"bytes"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestIOStreams_WithLogger_RoutesToErrOut(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	s := IOStreams{ErrOut: &buf}.WithLogger(l)

	s.Info("hello-info")
	s.Warn("hello-warn")
	s.Error("hello-error")
	s.Debug("hello-debug") // below Info level → suppressed

	out := buf.String()
	assert.Contains(t, out, "hello-info")
	assert.Contains(t, out, "hello-warn")
	assert.Contains(t, out, "hello-error")
	assert.NotContains(t, out, "hello-debug")
}

func TestIOStreams_NilLoggerIsNoOp(t *testing.T) {
	s := NewIOStreams() // no logger configured
	require.NotPanics(t, func() {
		s.Debug("d")
		s.Info("i")
		s.Warn("w")
		s.Error("e")
	})
}

func TestIOStreams_SeparateStreamsAreIsolated(t *testing.T) {
	var bufA, bufB bytes.Buffer
	a := IOStreams{ErrOut: &bufA}.WithLogger(slog.New(slog.NewTextHandler(&bufA, nil)))
	b := IOStreams{ErrOut: &bufB}.WithLogger(slog.New(slog.NewTextHandler(&bufB, nil)))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			a.Info("alpha")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			b.Info("bravo")
		}
	}()
	wg.Wait()

	assert.Contains(t, bufA.String(), "alpha")
	assert.Contains(t, bufB.String(), "bravo")
	assert.NotContains(t, bufA.String(), "bravo", "stream A must not receive B's logs")
	assert.NotContains(t, bufB.String(), "alpha", "stream B must not receive A's logs")
}
