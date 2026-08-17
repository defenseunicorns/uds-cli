// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package iostreams

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIOStreams(t *testing.T) {
	streams := NewIOStreams()

	assert.Equal(t, os.Stdin, streams.In())
	assert.NotNil(t, streams.Out())
	assert.NotNil(t, streams.ErrOut())
}

func TestNewTestIOStreams(t *testing.T) {
	streams, in, out, errOut := NewTestIOStreams()

	assert.NotNil(t, streams.In())
	assert.NotNil(t, streams.Out())
	assert.NotNil(t, streams.ErrOut())

	testData := "test data"

	in.WriteString(testData)
	assert.Equal(t, testData, in.String())

	out.WriteString(testData)
	assert.Equal(t, testData, out.String())

	errOut.WriteString(testData)
	assert.Equal(t, testData, errOut.String())

	streams.In().(*bytes.Buffer).WriteString("input")
	assert.Equal(t, testData+"input", in.String())
}

func TestIOStreams_WithLogger_RoutesToErrOut(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	s := New(nil, nil, &buf).WithLogger(l, nil)

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
	a := New(nil, nil, &bufA).WithLogger(slog.New(slog.NewTextHandler(&bufA, nil)), nil)
	b := New(nil, nil, &bufB).WithLogger(slog.New(slog.NewTextHandler(&bufB, nil)), nil)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 200 {
			a.Info("alpha")
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			b.Info("bravo")
		}
	}()
	wg.Wait()

	assert.Contains(t, bufA.String(), "alpha")
	assert.Contains(t, bufB.String(), "bravo")
	assert.NotContains(t, bufA.String(), "bravo", "stream A must not receive B's logs")
	assert.NotContains(t, bufB.String(), "alpha", "stream B must not receive A's logs")
}

func TestSynchronize_Idempotent(t *testing.T) {
	var buf bytes.Buffer
	w := synchronize(&buf)
	w2 := synchronize(w)
	assert.Same(t, w.(*lockedWriter), w2.(*lockedWriter), "synchronize must be idempotent")
}

func TestSynchronize_NilPassthrough(t *testing.T) {
	assert.Nil(t, synchronize(nil), "synchronize(nil) must return nil")
	// nil fields → Out()/ErrOut() return io.Discard; direct writes must not panic.
	s := New(nil, nil, nil)
	require.NotPanics(t, func() {
		_, _ = s.Out().Write([]byte("test"))
		_, _ = s.ErrOut().Write([]byte("test"))
	})
}

func TestIOStreams_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	s := New(nil, nil, &buf)

	const goroutines = 50
	const writesEach = 100
	const tokenLen = 4 // "G00-" through "G49-"

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			token := fmt.Sprintf("G%02d-", id)
			for range writesEach {
				_, _ = fmt.Fprint(s.ErrOut(), token)
			}
		}(i)
	}
	wg.Wait()

	got := buf.String()
	assert.Len(t, got, goroutines*writesEach*tokenLen, "byte count must be exact")
	// Each goroutine's distinct token must appear exactly writesEach times.
	// A split write would produce a partial token string not matching any token,
	// causing the count to fall short.
	for i := range goroutines {
		token := fmt.Sprintf("G%02d-", i)
		assert.Equal(t, writesEach, strings.Count(got, token),
			"token %s must appear exactly %d times", token, writesEach)
	}
}

func TestErrOut_SharedLockWithLogger(t *testing.T) {
	// WithLogger copies the IOStreams value; the copy must share the same
	// *lockedWriter so concurrent raw ErrOut() writes and slog writes are
	// serialized by a single mutex. The race detector catches any violation.
	var buf bytes.Buffer
	s := New(nil, nil, &buf)
	bound := s.WithLogger(slog.New(slog.NewTextHandler(s.ErrOut(), nil)), nil)

	const n = 500
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range n {
			bound.Info("L")
		}
	}()
	go func() {
		defer wg.Done()
		for range n {
			_, _ = fmt.Fprint(s.ErrOut(), "X")
		}
	}()
	wg.Wait()
	got := buf.String()
	assert.Equal(t, n, strings.Count(got, "msg=L"), "slog lines must appear")
	assert.Equal(t, n, strings.Count(got, "X"), "raw writes must appear")
}

func TestWithLogger_SharesLevelVarAcrossCopies(t *testing.T) {
	// Re-leveling relies on the LevelVar being shared by pointer: setting it on one
	// copy must re-level the logger held by another. A nil level owns no LevelVar.
	lv := new(slog.LevelVar)
	lv.Set(slog.LevelInfo)
	s := IOStreams{}.WithLogger(slog.New(slog.NewTextHandler(nil, nil)), lv)

	cp := s // value copy handed off elsewhere
	require.Same(t, lv, cp.LogLevel(), "copies must share the same LevelVar pointer")

	cp.LogLevel().Set(slog.LevelDebug)
	assert.Equal(t, slog.LevelDebug, s.LogLevel().Level(), "re-leveling a copy re-levels the original")

	require.Nil(t, s.WithLogger(slog.New(slog.NewTextHandler(nil, nil)), nil).LogLevel(),
		"a nil level owns no LevelVar")
}
