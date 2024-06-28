//go:build engine

// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/defenseunicorns/uds-cli/src/pkg/engine/api/monitor"
	"github.com/defenseunicorns/uds-cli/src/pkg/engine/api/resources"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
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

	ctx := context.Background()
	cache, err := resources.NewCache(ctx)
	if err != nil {
		log.Fatalf("Failed to create cache: %v", err)
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/monitor/pepr/", monitor.Pepr)
		r.Get("/monitor/pepr/{stream}", monitor.Pepr)

		r.Get("/resources/pods", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
				return
			}

			sendData := func() {
				pods := cache.Pods.GetPods()
				data, err := json.Marshal(pods)
				if err != nil {
					fmt.Fprintf(w, "data: Error: %v\n\n", err)
					flusher.Flush()
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}

			sendData()

			for {
				select {
				case <-r.Context().Done():
					return
				case <-cache.Pods.Changes:
					sendData()
				}
			}

		})
	})

	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
	return nil
}
