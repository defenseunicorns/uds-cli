// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
)

// Registry is an in-memory OCI registry available for a test's lifetime.
type Registry struct {
	Host string
}

// NewRegistry starts an in-memory OCI registry and closes it when the test ends.
func NewRegistry(t *testing.T) Registry {
	t.Helper()

	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)
	return Registry{Host: strings.TrimPrefix(server.URL, "http://")}
}
