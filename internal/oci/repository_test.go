// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/errdef"
)

func TestNewRemoteRepository_RegistrySchemeNegotiation(t *testing.T) {
	tests := []struct {
		name           string
		newServer      func() *httptest.Server
		plainHTTP      bool
		skipTLSVerify  bool
		wantPlainHTTP  bool
		wantResolveErr error
	}{
		{
			name:          "uses HTTP for an HTTP registry when enabled",
			newServer:     func() *httptest.Server { return httptest.NewServer(registry.New()) },
			plainHTTP:     true,
			wantPlainHTTP: true,
		},
		{
			name:          "keeps HTTPS for a TLS registry when plain HTTP is enabled",
			newServer:     func() *httptest.Server { return httptest.NewTLSServer(registry.New()) },
			plainHTTP:     true,
			skipTLSVerify: true,
			wantPlainHTTP: false,
		},
		{
			name:           "does not negotiate HTTP when disabled",
			newServer:      func() *httptest.Server { return httptest.NewServer(registry.New()) },
			wantPlainHTTP:  false,
			wantResolveErr: http.ErrSchemeMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.newServer()
			t.Cleanup(server.Close)

			ref := strings.TrimPrefix(server.URL, "http://")
			ref = strings.TrimPrefix(ref, "https://") + "/test/bundle:v1"
			repo, err := NewRemoteRepository(t.Context(), ref, bundleinternal.ConfigOptions{
				PlainHTTP:     tt.plainHTTP,
				SkipTLSVerify: tt.skipTLSVerify,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantPlainHTTP, repo.PlainHTTP)
			if tt.wantResolveErr != nil {
				_, err = repo.Resolve(t.Context(), "missing")
				require.ErrorIs(t, err, tt.wantResolveErr)
				return
			}
			_, err = repo.Resolve(t.Context(), "missing")
			require.ErrorIs(t, err, errdef.ErrNotFound, "expected registry response, got: %v", err)
		})
	}
}

func TestNewRemoteRepository_RejectsUntrustedTLSWithoutSkipVerify(t *testing.T) {
	server := httptest.NewTLSServer(registry.New())
	t.Cleanup(server.Close)

	ref := strings.TrimPrefix(server.URL, "https://") + "/test/bundle:v1"
	_, err := NewRemoteRepository(t.Context(), ref, bundleinternal.ConfigOptions{PlainHTTP: true})

	require.Error(t, err)
	var unknownAuthorityError x509.UnknownAuthorityError
	require.ErrorAs(t, err, &unknownAuthorityError)
}
