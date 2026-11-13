// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cli

package bundle_test

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/cli"
	"github.com/defenseunicorns/uds-cli/internal/mode"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"
)

func TestPushCommandWithPrivateBearerRealm(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	artifact := createInspectArtifact(t)

	// Use distinct registry and token hostnames, with the token hostname
	// resolving to a loopback address.
	var tokenRequests atomic.Int32
	tokens := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		tokenRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"token":"test-token","expires_in":3600}`)
	}))
	t.Cleanup(tokens.Close)
	_, port, err := net.SplitHostPort(tokens.Listener.Addr().String())
	require.NoError(t, err)
	realm := "https://localhost:" + port + "/token"

	storage := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm=%q,service="test-registry",scope="repository:bundles/private-auth:pull,push"`, realm))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		storage.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	ref := strings.TrimPrefix(server.URL, "https://") + "/bundles/private-auth:1.0.0"
	streams, _, out, _ := iostreams.NewTestIOStreams()
	root := cli.NewRootCommand(streams, mode.FeatureSet{})
	root.SetArgs([]string{"bundle", "push", artifact, ref, "--skip-tls-verify", "--uds-cache", t.TempDir()})
	require.NoError(t, root.ExecuteContext(t.Context()))
	require.Contains(t, out.String(), ref)
	require.Positive(t, tokenRequests.Load(), "push must authenticate through the split-host realm")

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/v2/bundles/private-auth/manifests/1.0.0", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
