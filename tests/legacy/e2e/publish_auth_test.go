package test

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"
)

func TestPublishWithPrivateBearerRealm(t *testing.T) {
	t.Setenv("CLI_FEATURES", "NextMode=false")
	t.Setenv("DOCKER_CONFIG", t.TempDir())

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
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm=%q,service="test-registry",scope="repository:bundles/real-simple:pull,push"`, realm))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		storage.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	packageDir := filepath.Join(dir, "packages", "no-cluster", "real-simple")
	bundleDir := filepath.Join(dir, "bundles", "11-real-simple")
	require.NoError(t, os.CopyFS(packageDir, os.DirFS("testdata/legacy/packages/no-cluster/real-simple")))
	require.NoError(t, os.CopyFS(bundleDir, os.DirFS("testdata/legacy/bundles/11-real-simple")))

	run := func(args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		output, err := exec.CommandContext(ctx, e2e.UDSBinPath, args...).CombinedOutput()
		require.NoError(t, err, "%s", output)
	}
	run("zarf", "package", "create", packageDir, "--confirm", "--output", packageDir)
	run("create", bundleDir, "--confirm", "--output", dir)
	archive := filepath.Join(dir, "uds-bundle-real-simple-"+e2e.Arch+"-0.0.1.tar.zst")
	host := strings.TrimPrefix(server.URL, "https://")
	run("publish", archive, host+"/bundles", "--insecure")
	require.Positive(t, tokenRequests.Load(), "publish must authenticate through the split-host realm")

	// Verify the bundle's published index through the authenticated registry.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/v2/bundles/real-simple/manifests/0.0.1", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
