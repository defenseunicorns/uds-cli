// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package monitor

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/defenseunicorns/uds-cli/src/pkg/engine/pepr"
	"github.com/defenseunicorns/uds-cli/src/pkg/engine/stream"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/go-chi/chi/v5"
)

func Pepr(w http.ResponseWriter, r *http.Request) {
	streamFilter := chi.URLParam(r, "stream")

	if !pepr.IsValidStreamFilter(pepr.StreamKind(streamFilter)) {
		http.Error(w, http.StatusText(404), 404)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set the headers for streaming
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Create a new BufferWriter
	bufferWriter := newBufferWriter(w)

	// pass context to stream reader to clean up spawned goroutines that watch pepr pods
	peprReader := pepr.NewStreamReader("", "")
	peprReader.JSON = true
	peprReader.FilterStream = pepr.StreamKind(streamFilter)

	peprStream := stream.NewStream(bufferWriter, peprReader, "pepr-system")
	peprStream.Follow = true
	peprStream.Timestamps = true

	// Start the stream in a goroutine
	go peprStream.Start(ctx)

	for {
		select {
		// Check if the client has disconnected
		case <-r.Context().Done():
			return

		// Flush every second if there is data
		case <-time.After(1 * time.Second):
			if bufferWriter.buffer.Len() > 0 {
				if err := bufferWriter.Flush(w); err != nil {
					message.WarnErr(err, "Failed to flush buffer")
					return
				}
			}
		}
	}
}

// bufferWriter is a custom writer that aggregates data and writes it to an http.ResponseWriter
type bufferWriter struct {
	buffer  *bytes.Buffer
	mutex   sync.Mutex
	flusher http.Flusher
}

// newBufferWriter creates a new BufferWriter
func newBufferWriter(w http.ResponseWriter) *bufferWriter {
	// Ensure the ResponseWriter also implements http.Flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		panic("ResponseWriter does not implement http.Flusher")
	}
	return &bufferWriter{
		buffer:  new(bytes.Buffer),
		flusher: flusher,
	}
}

// Write writes data to the buffer
func (bw *bufferWriter) Write(p []byte) (n int, err error) {
	bw.mutex.Lock()
	defer bw.mutex.Unlock()

	event := fmt.Sprintf("data: %s\n\n", p)

	// Write data to the buffer
	return bw.buffer.WriteString(event)
}

// Flush writes the buffer content to the http.ResponseWriter and flushes it
func (bw *bufferWriter) Flush(w http.ResponseWriter) error {
	bw.mutex.Lock()
	defer bw.mutex.Unlock()

	_, err := w.Write(bw.buffer.Bytes())
	if err != nil {
		return err
	}

	// Clear the buffer
	bw.buffer.Reset()

	// Flush the response
	bw.flusher.Flush()
	return nil
}
