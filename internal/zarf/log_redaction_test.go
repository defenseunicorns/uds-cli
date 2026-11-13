// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"strings"
	"testing"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactStandardLogsConcurrent(t *testing.T) {
	for _, first := range []int{0, 1} {
		t.Run(fmt.Sprint(first), func(t *testing.T) {
			var output bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&output)
			t.Cleanup(func() { log.SetOutput(previous) })
			started, done := make(chan struct{}, 2), make(chan struct{}, 2)
			release := []chan struct{}{make(chan struct{}), make(chan struct{})}
			for i, stop := range release {
				go func() {
					func() {
						defer redactStandardLogs([]string{"shared-secret", fmt.Sprintf("secret-%d", i)})()
						started <- struct{}{}
						<-stop
						log.Print("shared-secret secret-0 secret-1 ordinary")
					}()
					done <- struct{}{}
				}()
			}
			// Both calls enter before either returns; backend execution remains parallel.
			<-started
			<-started
			close(release[first])
			<-done
			log.Print("shared-secret secret-0 secret-1 ordinary")
			close(release[1-first])
			<-done
			assert.Equal(t, 3, strings.Count(output.String(), "*** *** *** ordinary"))
			assert.NotContains(t, output.String(), "secret")
			require.Same(t, &output, log.Writer())
			log.Print("restored")
			assert.Contains(t, output.String(), "restored")
		})
	}
}

func TestRedactStandardLogsRestoresAfterPanic(t *testing.T) {
	previous := log.Writer()
	require.Panics(t, func() {
		redactor := backendLogRedactor{secrets: []string{"secret"}}
		_ = redactor.Run(func() error { panic("backend failed") })
	})
	assert.Equal(t, previous, log.Writer())
}

func TestSensitiveLogValues(t *testing.T) {
	cfg := &UDSBundleConfig{
		Variables: bundleinternal.Variables{
			"private": bundleinternal.Variables{"items": []any{"", "secret", "secret-long", true, float64(42), nil}},
			"public":  "visible",
		},
		SensitiveVariables: map[string]bool{"private": true, "public": false},
	}
	redactor := newBackendLogRedactor(cfg)
	secrets := redactor.secrets
	require.ElementsMatch(t, []string{"secret", "secret-long", "true", "42"}, secrets)
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })
	require.NoError(t, redactor.Run(func() error {
		log.Print("secret-long secret true 42 visible")
		return nil
	}))
	assert.Contains(t, output.String(), "*** *** *** *** visible")
	assert.NotContains(t, output.String(), "-long")
	// Empty scopes leave the writer alone.
	redactStandardLogs(nil)()
	assert.Same(t, &output, log.Writer())
}

func TestSecretLogWriterCountsOriginalBytes(t *testing.T) {
	var output bytes.Buffer
	w := secretLogWriter{&output, strings.NewReplacer("secret", "***")}
	n, err := w.Write([]byte("secret message"))
	require.NoError(t, err)
	assert.Equal(t, len("secret message"), n)
	assert.Equal(t, "*** message", output.String())
	reader, writer := io.Pipe()
	require.NoError(t, reader.Close())
	t.Cleanup(func() { _ = writer.Close() })
	w.next = writer
	n, err = w.Write([]byte("secret"))
	require.ErrorIs(t, err, io.ErrClosedPipe)
	assert.Zero(t, n)
}
