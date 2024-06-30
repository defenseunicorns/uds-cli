//go:build engine

// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package api

import (
	"context"
	"log"
	"net/http"

	"github.com/defenseunicorns/uds-cli/src/pkg/engine/api/monitor"
	"github.com/defenseunicorns/uds-cli/src/pkg/engine/api/resources"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
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

		r.Get("/resources/namespaces", resources.Bind[*v1.Namespace](cache.Namespaces.GetResources, cache.Namespaces.Changes))
		r.Get("/resources/pods", resources.Bind[*v1.Pod](cache.Pods.GetResources, cache.Pods.Changes))
		r.Get("/resources/deployments", resources.Bind[*appsv1.Deployment](cache.Deployments.GetResources, cache.Deployments.Changes))
		r.Get("/resources/daemonsets", resources.Bind[*appsv1.DaemonSet](cache.Daemonsets.GetResources, cache.Daemonsets.Changes))
		r.Get("/resources/statefulsets", resources.Bind[*appsv1.StatefulSet](cache.Statefulsets.GetResources, cache.Statefulsets.Changes))

	})

	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
	return nil
}
