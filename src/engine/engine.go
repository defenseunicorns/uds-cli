//go:build engine

// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package engine

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"

	"github.com/fatih/color"
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
			cmd := exec.Command("ping", "google.com")
			//output, err := runCommand("npx", "pepr", "monitor")
			streamCommandOutput(cmd, w, r)
		})
	})

	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
	return nil
}

func streamCommandOutput(cmd *exec.Cmd, w http.ResponseWriter, r *http.Request) {
	// Set the headers for streaming
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		handleError(w, err, "Failed to get command output", http.StatusInternalServerError)
		return
	}

	if err := cmd.Start(); err != nil {
		// todo: need cmd.Wait() after calling Start? maybe when returning?
		handleError(w, err, "Failed to start command", http.StatusInternalServerError)
		return
	}

	// Create a buffered reader to read the command output
	reader := bufio.NewReader(stdout)

	connClosed := r.Context().Done()

	// Stream the output line by line
	for {
		select {
		case <-connClosed:
			log.Println("Client closed connection")
			if err := cmd.Process.Kill(); err != nil {
				log.Printf("Failed to kill process: %v", err)
			}
			return
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			styledLine := pterm.Color(color.FgHiMagenta).Sprintf(strings.TrimSpace(line))
			fmt.Println(styledLine)

			// Write the data as an SSE message
			fmt.Fprintf(w, "data: %s\n\n", styledLine)
			w.(http.Flusher).Flush() // Flush the buffer to ensure the client receives the data in real-time
		}
	}

	if err := cmd.Wait(); err != nil {
		log.Printf("Command finished with error: %v", err)
	}
}

// handleError sends an error response with a given status code and logs the error
func handleError(w http.ResponseWriter, err error, message string, statusCode int) {
	log.Printf("error: %v - %s", err, message)
	http.Error(w, message, statusCode)
}
