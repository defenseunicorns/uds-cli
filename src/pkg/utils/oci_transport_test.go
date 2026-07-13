// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package utils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNegotiatePlainHTTPForOCIRef(t *testing.T) {
	t.Run("insecure false does not negotiate", func(t *testing.T) {
		got, err := NegotiatePlainHTTPForOCIRef(context.Background(), "oci://example.com/repo:tag", false)
		require.NoError(t, err)
		require.False(t, got)
	})

	t.Run("https registry remains https even when auth is required", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/v2/", r.URL.Path)
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()

		got, err := NegotiatePlainHTTPForOCIRef(context.Background(), "oci://"+strings.TrimPrefix(srv.URL, "https://")+"/repo:tag", true)
		require.NoError(t, err)
		require.False(t, got)
	})

	t.Run("plain http registry negotiates to plain http", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/v2/", r.URL.Path)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		got, err := NegotiatePlainHTTPForOCIRef(context.Background(), "oci://"+strings.TrimPrefix(srv.URL, "http://")+"/repo:tag", true)
		require.NoError(t, err)
		require.True(t, got)
	})
}

func TestNegotiatePlainHTTPForRegistry(t *testing.T) {
	t.Run("explicit https registry remains https", func(t *testing.T) {
		got, err := NegotiatePlainHTTPForRegistry(context.Background(), "https://registry.example.com", true)
		require.NoError(t, err)
		require.False(t, got)
	})

	t.Run("explicit http registry uses plain http", func(t *testing.T) {
		got, err := NegotiatePlainHTTPForRegistry(context.Background(), "http://registry.example.com", true)
		require.NoError(t, err)
		require.True(t, got)
	})
}
