// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicIOStreamsContracts(t *testing.T) {
	t.Parallel()

	input := bytes.NewBufferString("input")
	var output, diagnostics bytes.Buffer
	streams := iostreams.New(input, &output, &diagnostics)
	got, err := io.ReadAll(streams.In())
	require.NoError(t, err)
	assert.Equal(t, "input", string(got))
	_, err = fmt.Fprint(streams.Out(), "output")
	require.NoError(t, err)
	_, err = fmt.Fprint(streams.ErrOut(), "diagnostic")
	require.NoError(t, err)
	assert.Equal(t, "output", output.String())
	assert.Equal(t, "diagnostic", diagnostics.String())

	testStreams, testIn, testOut, testErrOut := iostreams.NewTestIOStreams()
	testIn.WriteString("test input")
	_, err = fmt.Fprint(testStreams.Out(), "test output")
	require.NoError(t, err)
	_, err = fmt.Fprint(testStreams.ErrOut(), "test diagnostics")
	require.NoError(t, err)
	got, err = io.ReadAll(testStreams.In())
	require.NoError(t, err)
	assert.Equal(t, "test input", string(got))
	assert.Equal(t, "test output", testOut.String())
	assert.Equal(t, "test diagnostics", testErrOut.String())

	var logs bytes.Buffer
	level := new(slog.LevelVar)
	level.Set(slog.LevelDebug)
	logged := iostreams.New(nil, nil, nil).WithLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: level})), level)
	require.Same(t, level, logged.LogLevel())
	logged.Debug("debug")
	logged.Info("info")
	logged.Warn("warn")
	logged.Error("error")
	for _, message := range []string{"debug", "info", "warn", "error"} {
		assert.Contains(t, logs.String(), message)
	}

	zero := iostreams.IOStreams{}
	require.NotPanics(t, func() {
		_, _ = fmt.Fprint(zero.Out(), "output")
		_, _ = fmt.Fprint(zero.ErrOut(), "diagnostic")
		zero.Debug("debug")
		zero.Info("info")
		zero.Warn("warn")
		zero.Error("error")
	})
}

type recordingHandler struct {
	slog.Handler
	handled atomic.Bool
}

func (h *recordingHandler) Handle(ctx context.Context, record slog.Record) error {
	h.handled.Store(true)
	return h.Handler.Handle(ctx, record)
}

func TestPublicIOStreamsCallerLoggerReachesBundleOperation(t *testing.T) {
	loggerHandler := &recordingHandler{Handler: slog.NewTextHandler(io.Discard, nil)}
	callerLogger := slog.New(loggerHandler)
	streams := iostreams.New(nil, nil, nil).WithLogger(callerLogger, nil)

	require.Same(t, callerLogger, streams.Logger())
	require.Nil(t, streams.LogLevel())

	artifact := createLibraryArtifact(t)
	_, err := bundle.Inspect(t.Context(), bundle.InspectOptions{
		Source:                    artifact,
		Config:                    libraryFixtureConfig(t),
		SkipSignatureVerification: true,
		Streams:                   streams,
	})
	require.NoError(t, err)
	assert.True(t, loggerHandler.handled.Load())
}
