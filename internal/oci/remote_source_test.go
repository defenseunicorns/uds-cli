// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	orasregistry "oras.land/oras-go/v2/registry"
)

func TestRemoteReadSourceCacheReuse(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			var downloads atomic.Int64
			var blocked atomic.Bool
			handler := registry.New()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") {
					downloads.Add(1)
					if blocked.Load() {
						http.Error(w, "blob downloads blocked", http.StatusForbidden)
						return
					}
				}
				handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			ref := strings.TrimPrefix(server.URL, "http://") + "/test/bundle:v1"
			opts := bundleinternal.ConfigOptions{PlainHTTP: true}
			repo, err := NewRemoteRepository(t.Context(), ref, opts)
			require.NoError(t, err)
			data := []byte("shared package content")
			desc, err := PushBytes(t.Context(), repo, "application/octet-stream", data, nil)
			require.NoError(t, err)
			cacheDir := filepath.Join(t.TempDir(), "cache")
			if enabled {
				opts.CacheDir = cacheDir
			}
			source, err := NewRemoteReadSource(t.Context(), ref, opts, iostreams.IOStreams{})
			require.NoError(t, err)
			got, err := FetchBytes(t.Context(), source, desc)
			require.NoError(t, err)
			assert.Equal(t, data, got)
			require.EqualValues(t, 1, downloads.Load())
			if enabled {
				require.FileExists(t, filepath.Join(cacheDir, "bundle-oci", "images", "blobs", "sha256", desc.Digest.Encoded()))
				assert.NoDirExists(t, filepath.Join(cacheDir, "images"))
				assert.NoDirExists(t, filepath.Join(cacheDir, "layers"))
				blocked.Store(true)
			}
			// A fresh client must see the persisted cache, not just reuse an
			// in-memory reader held by the first client.
			warm, err := NewRemoteReadSource(t.Context(), ref, opts, iostreams.IOStreams{})
			require.NoError(t, err)
			exists, err := warm.Exists(t.Context(), desc)
			require.NoError(t, err)
			assert.True(t, exists)
			got, err = FetchBytes(t.Context(), warm, desc)
			require.NoError(t, err)
			assert.Equal(t, data, got)
			if enabled {
				assert.EqualValues(t, 1, downloads.Load())
			} else {
				assert.EqualValues(t, 2, downloads.Load())
				assert.NoDirExists(t, cacheDir)
			}
		})
	}
}

func TestRemoteReadSourceKeepsRegistryDiscoveryLive(t *testing.T) {
	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)
	ref := strings.TrimPrefix(server.URL, "http://") + "/test/bundle:v1"
	opts := bundleinternal.ConfigOptions{PlainHTTP: true, CacheDir: t.TempDir()}
	repo, err := NewRemoteRepository(t.Context(), ref, opts)
	require.NoError(t, err)
	pushIndex := func(version string) ocispec.Descriptor {
		t.Helper()
		data, err := json.Marshal(ocispec.Index{
			MediaType:   ocispec.MediaTypeImageIndex,
			Annotations: map[string]string{"version": version},
		})
		require.NoError(t, err)
		desc := NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, data)
		require.NoError(t, PushReferenceBytes(t.Context(), repo, desc, data, "v1"))
		return desc
	}
	first := pushIndex("first")
	source, err := NewRemoteReadSource(t.Context(), ref, opts, iostreams.IOStreams{})
	require.NoError(t, err)
	_, err = FetchBytes(t.Context(), source, first)
	require.NoError(t, err)
	second := pushIndex("second")
	// Even when the first index is cached, a mutable tag must resolve to
	// current registry content. Reference fetching must do the same.
	resolved, err := source.Resolve(t.Context(), "v1")
	require.NoError(t, err)
	assert.Equal(t, second.Digest, resolved.Digest)
	fetcher, ok := source.(orasregistry.ReferenceFetcher)
	require.True(t, ok)
	fetched, reader, err := fetcher.FetchReference(t.Context(), "v1")
	require.NoError(t, err)
	_, err = content.ReadAll(reader, fetched)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	assert.Equal(t, second.Digest, fetched.Digest)
	require.FileExists(t, filepath.Join(opts.CacheDir, "bundle-oci", "images", "blobs", "sha256", second.Digest.Encoded()))

	// Signature evidence is outside the bundle's forward content graph.
	// The cache adapter must not hide registry referrer discovery.
	evidence := []byte("signature evidence")
	layer, err := PushBytes(t.Context(), repo, MediaTypeBundleSignature, evidence, nil)
	require.NoError(t, err)
	signature, err := oras.PackManifest(t.Context(), repo, oras.PackManifestVersion1_1, MediaTypeBundleSignature, oras.PackManifestOptions{
		Subject: &second,
		Layers:  []ocispec.Descriptor{layer},
	})
	require.NoError(t, err)
	graph, ok := source.(content.ReadOnlyGraphStorage)
	require.True(t, ok)
	refs, err := orasregistry.Referrers(t.Context(), graph, second, MediaTypeBundleSignature)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, signature.Digest, refs[0].Digest)
	predecessors, err := graph.Predecessors(t.Context(), second)
	require.NoError(t, err)
	assert.Contains(t, predecessors, refs[0])
	_, writable := source.(oras.Target)
	assert.False(t, writable, "a read source must not expose publication methods")
}

func TestPullBundleReusesRemoteReadSource(t *testing.T) {
	var blocked atomic.Bool
	var downloads atomic.Int64
	handler := registry.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && (strings.Contains(r.URL.Path, "/blobs/") || strings.Contains(r.URL.Path, "/manifests/sha256:")) {
			downloads.Add(1)
			if blocked.Load() {
				http.Error(w, "immutable downloads blocked", http.StatusForbidden)
				return
			}
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	ref := strings.TrimPrefix(server.URL, "http://") + "/test/bundle:v1"
	cfg := newTestConfig()
	cfg.Options.PlainHTTP = true
	cfg.Options.CacheDir = t.TempDir()
	cfg.Options.TmpDir = t.TempDir()
	repo, err := NewRemoteRepository(t.Context(), ref, *cfg.Options)
	require.NoError(t, err)
	payload := []byte("bundle graph content")
	layer, err := PushBytes(t.Context(), repo, "application/octet-stream", payload, nil)
	require.NoError(t, err)
	manifest, err := PackBundleDefinitionManifest(t.Context(), repo, []ocispec.Descriptor{layer})
	require.NoError(t, err)
	indexBytes, err := json.Marshal(ocispec.Index{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageIndex,
		ArtifactType: MediaTypeBundle,
		Manifests:    []ocispec.Descriptor{manifest},
		Annotations:  map[string]string{AnnotationBundleArchitecture: cfg.Options.Architecture},
	})
	require.NoError(t, err)
	child := NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, indexBytes)
	require.NoError(t, PushReferenceBytes(t.Context(), repo, child, indexBytes, "v1"))
	evidence := []byte("signature evidence")
	require.NoError(t, PublishBundleSignature(t.Context(), repo, repo, child, evidence, false))
	pullOpts := PullOptions{
		Config: cfg,
		PullHooks: PullHooks{
			VerifyBundle: func(_ context.Context, index, signature []byte) error {
				assert.Equal(t, indexBytes, index)
				assert.Equal(t, evidence, signature)
				return nil
			},
			CreateBundleArchive: func(_ context.Context, _ iostreams.IOStreams, ociDir, targetDir string, _ ocispec.Index, _ string) (string, error) {
				got, err := os.ReadFile(filepath.Join(ociDir, "blobs", "sha256", layer.Digest.Encoded()))
				require.NoError(t, err)
				assert.Equal(t, payload, got)
				got, err = os.ReadFile(filepath.Join(ociDir, "index.json"))
				require.NoError(t, err)
				assert.Equal(t, indexBytes, got)
				got, err = os.ReadFile(filepath.Join(filepath.Dir(ociDir), BundleSignatureFileName))
				require.NoError(t, err)
				assert.Equal(t, evidence, got)
				return filepath.Join(targetDir, "bundle.tar.zst"), nil
			},
		},
	}
	_, err = NewDefaultPuller().PullBundle(t.Context(), ref, t.TempDir(), pullOpts)
	require.NoError(t, err)
	require.Positive(t, downloads.Load())
	downloads.Store(0)
	blocked.Store(true)
	_, err = NewDefaultPuller().PullBundle(t.Context(), ref, t.TempDir(), pullOpts)
	require.NoError(t, err)
	assert.Zero(t, downloads.Load(), "index, graph, and signature reads must retain the cached source")
}

func TestRemoteReadSourceTransport(t *testing.T) {
	for _, tls := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "https"}[tls], func(t *testing.T) {
			var server *httptest.Server
			if tls {
				server = httptest.NewTLSServer(registry.New())
			} else {
				server = httptest.NewServer(registry.New())
			}
			t.Cleanup(server.Close)
			ref := strings.TrimPrefix(strings.TrimPrefix(server.URL, "http://"), "https://") + "/test/bundle:v1"
			source, err := NewRemoteReadSource(t.Context(), ref, bundleinternal.ConfigOptions{PlainHTTP: true, SkipTLSVerify: tls}, iostreams.IOStreams{})
			require.NoError(t, err)
			_, err = source.Resolve(t.Context(), "missing")
			assert.True(t, IsNotFound(err), "expected registry response, got %v", err)
		})
	}
}

func TestRemoteReadSourceCacheDiagnostics(t *testing.T) {
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo} {
		t.Run(level.String(), func(t *testing.T) {
			streams, _, out, errOut := iostreams.NewTestIOStreams()
			streams = streams.WithLogger(slog.New(slog.NewJSONHandler(streams.ErrOut(), &slog.HandlerOptions{Level: level})), nil)
			opts := bundleinternal.ConfigOptions{CacheDir: t.TempDir()}
			_, err := NewRemoteReadSource(t.Context(), "registry.example.com/test/bundle:v1", opts, streams)
			require.NoError(t, err)
			assert.Empty(t, out.String())
			if level == slog.LevelInfo {
				assert.Empty(t, errOut.String())
				return
			}
			assert.Contains(t, errOut.String(), "initializing bundle OCI cache")
			assert.Contains(t, errOut.String(), "bundle OCI cache initialized")
			assert.Contains(t, errOut.String(), filepath.Join(opts.CacheDir, "bundle-oci", "images"))
		})
	}
	t.Run("initialization failure", func(t *testing.T) {
		cacheDir := filepath.Join(t.TempDir(), "not-a-directory")
		require.NoError(t, os.WriteFile(cacheDir, []byte("blocked"), 0o600))
		streams, _, _, errOut := iostreams.NewTestIOStreams()
		streams = streams.WithLogger(slog.New(slog.NewTextHandler(streams.ErrOut(), &slog.HandlerOptions{Level: slog.LevelDebug})), nil)
		source, err := NewRemoteReadSource(t.Context(), "registry.example.com/test/bundle:v1", bundleinternal.ConfigOptions{CacheDir: cacheDir}, streams)
		require.Error(t, err)
		assert.Nil(t, source)
		assert.NotContains(t, errOut.String(), "cache initialized")
	})
}
