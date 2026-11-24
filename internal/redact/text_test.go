// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package redact

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretLogWriterCountsOriginalBytes(t *testing.T) {
	var output bytes.Buffer
	w := Writer(&output, Text([]string{"secret"}))
	n, err := w.Write([]byte("secret message"))
	require.NoError(t, err)
	assert.Equal(t, len("secret message"), n)
	assert.Equal(t, "*** message", output.String())
	reader, writer := io.Pipe()
	require.NoError(t, reader.Close())
	t.Cleanup(func() { _ = writer.Close() })
	w = Writer(writer, Text([]string{"secret"}))
	n, err = w.Write([]byte("secret"))
	require.ErrorIs(t, err, io.ErrClosedPipe)
	assert.Zero(t, n)
}

func TestQuotedValue(t *testing.T) {
	value := "line\nbreak"
	mask := Text([]string{value, ""})
	assert.Equal(t, `path "***"`, mask.Replace(fmt.Sprintf("path %q", value)))
}

func TestOperationKeepsLoggerAndSafeErrorIdentities(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	var logs bytes.Buffer
	streams = streams.WithLogger(slog.New(slog.NewJSONHandler(&logs, nil)), nil)
	_, scope := Bind(t.Context(), &streams)
	log := streams.Logger().WithGroup("bundle").With("early", "private-value")
	scope.Add("private-value")
	log.Info("private-value", "nested", slog.GroupValue(slog.String("name", "private-value")))
	failure := fmt.Errorf("open private-value: %w", context.Canceled)
	failure = scope.Error(failure)
	require.ErrorIs(t, failure, context.Canceled)
	require.NotContains(t, failure.Error(), "private-value")
	assert.Contains(t, logs.String(), `"bundle":{"early":"***","nested":{"name":"***"}}`)
	assert.NotContains(t, logs.String(), "private-value")
}

func TestMaskedSentinelRetainsIdentity(t *testing.T) {
	_, scope := WithScope(t.Context())
	scope.Add("bundle")
	sentinel := errors.New("creating bundle")
	err := scope.Error(fmt.Errorf("operation failed: %w", sentinel))
	require.ErrorIs(t, err, sentinel)
	assert.EqualError(t, err, "operation failed: creating ***")
}

func TestNestedOperationProtectsIndependentStreamsAndErrors(t *testing.T) {
	outer, _, _, outerLog := iostreams.NewTestIOStreams()
	ctx, scope := Bind(t.Context(), &outer)
	scope.Add("private-value")
	inner, _, _, innerLog := iostreams.NewTestIOStreams()
	_, nested := Bind(ctx, &inner)
	require.Same(t, scope, nested)
	_, err := fmt.Fprint(inner.ErrOut(), "private-value")
	require.NoError(t, err)
	innerErr := nested.Error(errors.New("inner: private-value"))
	assert.Equal(t, "***", innerLog.String())
	assert.Empty(t, outerLog.String())
	assert.EqualError(t, innerErr, "inner: ***")
}

func TestNestedMaskPreservesTypedErrorAndNewMarks(t *testing.T) {
	_, inner := WithScope(t.Context())
	inner.Add("private-version")
	original := &spec.UnsupportedBundleAPIVersionError{Actual: "private-version", Expected: "later-mark"}
	innerErr := inner.Error(fmt.Errorf("validation: %w", errors.Join(original, context.Canceled)))

	_, outer := WithScope(t.Context())
	outer.Add("later-mark")
	err := outer.Error(innerErr)
	var typed *spec.UnsupportedBundleAPIVersionError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, original.Actual, typed.Actual)
	require.Equal(t, original.Expected, typed.Expected)
	require.EqualError(t, typed, `uds.bundle_api_version "***" is not supported; expected "***"`)
	require.ErrorIs(t, err, context.Canceled)
}
