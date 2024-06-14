//go:build engine

// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package engine

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/defenseunicorns/uds-cli/src/pkg/engine/pepr"
	"github.com/defenseunicorns/uds-cli/src/pkg/engine/stream"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/pterm/pterm"
)

func Start() error {
	r := chi.NewRouter()

	// CORS middleware setup
	cors := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173"}, // Change to the address of your Svelte app
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type"},
		ExposedHeaders:   []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	})
	r.Use(cors.Handler)

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/policies", func(w http.ResponseWriter, r *http.Request) {

			var buf bytes.Buffer
			logStream := io.MultiWriter(&buf, os.Stderr)
			pterm.SetDefaultOutput(logStream)

			ctx, cancel := context.WithCancel(context.Background())

			// pass context to stream reader to clean up spawned goroutines that watch pepr pods
			peprReader := pepr.NewStreamReader(ctx, true, "", "")
			peprStream := stream.NewStream(logStream, peprReader, "pepr-system")
			peprStream.Follow = true

			// Start the stream in a goroutine
			go peprStream.Start()

			// Stream the output to the client
			streamPeprOutput(&buf, w, r)

			// Cancel the context when the client disconnects
			cancel()
			log.Println("after cancel")
		})
	})

	r.Route("/", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			foo := fmt.Sprintf("Number of goroutines: %d", runtime.NumGoroutine())
			w.Write([]byte(foo))
		})
	})

	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
	return nil
}

func streamPeprOutput(buf *bytes.Buffer, w http.ResponseWriter, r *http.Request) {
	// Set the headers for streaming
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	connClosed := r.Context().Done()

	reader := bufio.NewReader(buf)
	for {
		select {
		case <-connClosed:
			log.Println("Client closed connection")
			return
		default:
			log.Printf("Number of goroutines: %d", runtime.NumGoroutine())

			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					message.ErrorWebf(err, w, "Unable to read the stream")
					return
				}
				// Sleep briefly to wait for more data
				// todo: move this to outside the if statement
				time.Sleep(500 * time.Millisecond)
				continue
			}

			trimmed := strings.TrimSpace(line)
			if len(trimmed) > 0 {
				fmt.Fprintf(w, "data: %s\n\n", trimmed)
				w.(http.Flusher).Flush()
			}
		}
	}
}

// handleError sends an error response with a given status code and logs the error
func handleError(w http.ResponseWriter, err error, message string, statusCode int) {
	log.Printf("error: %v - %s", err, message)
	http.Error(w, message, statusCode)
}
