// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package utils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry"
)

func TestNegotiatePlainHTTP(t *testing.T) {
	originalInsecure := config.CommonOptions.Insecure
	t.Cleanup(func() {
		config.CommonOptions.Insecure = originalInsecure
	})

	t.Run("insecure false does not negotiate", func(t *testing.T) {
		config.CommonOptions.Insecure = false
		ref, err := registry.ParseReference("example.com/repo:tag")
		require.NoError(t, err)

		got, err := NegotiatePlainHTTP(context.Background(), ref)
		require.NoError(t, err)
		require.False(t, got)
	})

	t.Run("https registry remains https even when auth is required", func(t *testing.T) {
		config.CommonOptions.Insecure = true
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/v2/", r.URL.Path)
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()

		ref, err := registry.ParseReference(strings.TrimPrefix(srv.URL, "https://") + "/repo:tag")
		require.NoError(t, err)
		got, err := NegotiatePlainHTTP(context.Background(), ref)
		require.NoError(t, err)
		require.False(t, got)
	})

	t.Run("plain http registry negotiates to plain http", func(t *testing.T) {
		config.CommonOptions.Insecure = true
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/v2/", r.URL.Path)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		ref, err := registry.ParseReference(strings.TrimPrefix(srv.URL, "http://") + "/repo:tag")
		require.NoError(t, err)
		got, err := NegotiatePlainHTTP(context.Background(), ref)
		require.NoError(t, err)
		require.True(t, got)
	})
}
