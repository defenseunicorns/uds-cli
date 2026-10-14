// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	packageoci "github.com/defenseunicorns/pkg/oci"
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
	"oras.land/oras-go/v2/errdef"
)

func TestRemoteSourceVerifyAndIngestFilteredRegistryPackage(t *testing.T) {
	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)

	pkgDir := t.TempDir()
	writeFilteredPackageFiles(t, pkgDir)
	pkgLayout, err := layout.LoadFromDir(t.Context(), pkgDir, layout.PackageLayoutOptions{VerificationStrategy: layout.VerifyNever})
	require.NoError(t, err)
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()

	ref := strings.TrimPrefix(server.URL, "http://") + "/test/filtered:1.0.0"
	remote, err := zoci.NewRemoteWithOptions(t.Context(), ref, ocispec.Platform{Architecture: "amd64", OS: packageoci.MultiOS}, zoci.RemoteClientOptions{RemoteOptions: zarfTypes.RemoteOptions{PlainHTTP: true}})
	require.NoError(t, err)
	_, err = remote.PushPackage(t.Context(), pkgLayout, zoci.PublishOptions{Retries: 1, OCIConcurrency: 1})
	require.NoError(t, err)

	store, err := udsoci.CreateStore(t.TempDir())
	require.NoError(t, err)
	source := &remoteSource{
		ref:  ref,
		arch: "amd64",
		opts: bundleinternal.ConfigOptions{PlainHTTP: true, TmpDir: t.TempDir(), Concurrency: 1},
	}
	rootDesc, err := remote.ResolveRoot(t.Context())
	require.NoError(t, err)
	resolved, err := source.resolveFilteredLayers(t.Context(), filters.Combine(filters.ForDeploy("included", false)))
	require.NoError(t, err)
	assert.Equal(t, rootDesc.Digest.String(), resolved.remote.Repo().Reference.Reference)
	assert.Equal(t, strings.TrimSuffix(ref, ":1.0.0")+"@"+rootDesc.Digest.String(), resolved.remote.Repo().Reference.String())
	loaded, err := source.LoadPackageSpec(t.Context(), filters.Combine(filters.ForDeploy("included", false)))
	require.NoError(t, err)
	assert.Equal(t, rootDesc.Digest.String(), loaded.Digest)
	assert.Equal(t, []string{"included"}, loaded.Components)
	loader := NewSourcePackageLayoutLoader(bundleinternal.ConfigOptions{Architecture: "amd64", PlainHTTP: true, TmpDir: t.TempDir(), Concurrency: 1}, t.TempDir())
	pkg := &spec.Package{Name: "pkg", Source: "oci://" + ref, OptionalComponents: []string{"included"}}
	intended, err := loader.LoadPackageSpec(t.Context(), pkg)
	require.NoError(t, err)
	assert.Equal(t, rootDesc, loader.resolvedRoots[pkg.Name])
	deploymentLayout, err := loader.LoadPackageLayout(t.Context(), pkg, t.TempDir(), LoadOptions{IsPartial: true})
	require.NoError(t, err)
	assert.Equal(t, intended.Digest, deploymentLayout.Layout.Digest())
	require.NoError(t, deploymentLayout.Layout.Cleanup())

	descs, err := source.VerifyAndIngestFiltered(t.Context(), t.TempDir(), layout.PackageLayoutOptions{
		Filter:               filters.Combine(filters.ForDeploy("included", false)),
		VerificationStrategy: layout.VerifyNever,
	}, store)
	require.NoError(t, err)
	require.Len(t, descs, 1)

	manifestBytes, err := udsoci.FetchBytes(t.Context(), store, descs[0])
	require.NoError(t, err)
	var manifest ocispec.Manifest
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
	titles := layerTitles(manifest.Layers)
	assert.Contains(t, titles, layout.ZarfYAML)
	assert.Contains(t, titles, filepath.ToSlash(filepath.Join(layout.ComponentsDir, "included.tar")))
	assert.NotContains(t, titles, filepath.ToSlash(filepath.Join(layout.ComponentsDir, "excluded.tar")))
}

func TestRemoteSourceNewZociRemote_RegistrySchemeNegotiation(t *testing.T) {
	server := httptest.NewTLSServer(registry.New())
	t.Cleanup(server.Close)

	ref := strings.TrimPrefix(server.URL, "https://") + "/test/package:v1"
	source := &remoteSource{
		ref:  ref,
		arch: "amd64",
		opts: bundleinternal.ConfigOptions{PlainHTTP: true, SkipTLSVerify: true},
	}

	remote, err := source.newZociRemote(t.Context())
	require.NoError(t, err)
	assert.False(t, remote.Repo().PlainHTTP)
	_, err = remote.Repo().Resolve(t.Context(), "missing")
	require.ErrorIs(t, err, errdef.ErrNotFound, "expected registry response, got: %v", err)
}

func TestRemoteSourceCacheFallbackDiscardsPartialDownloads(t *testing.T) {
	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)
	pkgDir := t.TempDir()
	writeFilteredPackageFiles(t, pkgDir)
	pkgLayout, err := layout.LoadFromDir(t.Context(), pkgDir, layout.PackageLayoutOptions{VerificationStrategy: layout.VerifyNever})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
	ref := strings.TrimPrefix(server.URL, "http://") + "/test/fallback:v1"
	remote, err := zoci.NewRemoteWithOptions(t.Context(), ref, ocispec.Platform{Architecture: "amd64", OS: packageoci.MultiOS}, zoci.RemoteClientOptions{RemoteOptions: zarfTypes.RemoteOptions{PlainHTTP: true}})
	require.NoError(t, err)
	_, err = remote.PushPackage(t.Context(), pkgLayout, zoci.PublishOptions{Retries: 1, OCIConcurrency: 1})
	require.NoError(t, err)
	streams, _, _, errOut := iostreams.NewTestIOStreams()
	streams = streams.WithLogger(slog.New(slog.NewTextHandler(streams.ErrOut(), nil)), nil)
	source := &remoteSource{
		ref: ref, arch: "amd64", streams: streams,
		opts: bundleinternal.ConfigOptions{CacheDir: t.TempDir(), PlainHTTP: true, Concurrency: 1},
	}
	filter := BuildComponentFilter([]string{"included"})
	// Warm only the metadata first so the obstruction below is encountered
	// during package downloading, after the first attempt owns a staging dir.
	spec, err := source.LoadPackageSpec(t.Context(), filter)
	require.NoError(t, err)
	ingest := filepath.Join(source.opts.CacheDir, "bundle-oci", "images", "ingest")
	if err := os.Remove(ingest); err != nil {
		require.ErrorIs(t, err, os.ErrNotExist)
	}
	require.NoError(t, os.WriteFile(ingest, []byte("blocked cache ingest"), 0o600))
	stage := t.TempDir()
	marker := filepath.Join(stage, "caller-owned")
	require.NoError(t, os.WriteFile(marker, []byte("keep"), 0o600))

	loaded, err := source.PullFiltered(t.Context(), stage, layout.PackageLayoutOptions{Filter: filter, VerificationStrategy: layout.VerifyNever})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, loaded.Cleanup()) })
	assert.Equal(t, spec.Digest, loaded.Digest(), "retry must keep the resolved package digest")
	assert.NotEmpty(t, source.opts.CacheDir)
	assert.FileExists(t, marker)
	assert.FileExists(t, filepath.Join(loaded.DirPath(), layout.ZarfYAML))
	assert.Equal(t, 1, strings.Count(errOut.String(), "retrying retrieval once without caching"))
	entries, err := os.ReadDir(stage)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "only the caller's marker and the successful download directory remain")
}

func TestRemoteSourceResolvedReference(t *testing.T) {
	root := ocispec.Descriptor{Digest: digest.FromString("resolved")}
	tests := []struct {
		name string
		ref  string
	}{
		{name: "tag", ref: "registry.example.com/team/package:latest"},
		{name: "digest", ref: "registry.example.com/team/package@" + root.Digest.String()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (&remoteSource{ref: tt.ref, resolvedRoot: &root}).resolvedReference()
			require.NoError(t, err)
			assert.Equal(t, "registry.example.com/team/package@"+root.Digest.String(), got)
		})
	}
}

func TestRemoteSourceCacheDiagnostics(t *testing.T) {
	tests := []struct {
		name     string
		existing bool
		disabled bool
		blocked  bool
		level    slog.Level
		noLogger bool
	}{
		{name: "new cache", level: slog.LevelDebug},
		{name: "existing cache", existing: true, level: slog.LevelDebug},
		{name: "disabled cache", disabled: true, level: slog.LevelDebug},
		{name: "initialization failure", blocked: true, level: slog.LevelDebug},
		{name: "quiet at info level", level: slog.LevelInfo},
		{name: "no logger", noLogger: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheDir := filepath.Join(t.TempDir(), "cache")
			storePath := filepath.Join(cacheDir, "bundle-oci", zoci.ImageCacheDirectory)
			if tt.existing {
				require.NoError(t, os.MkdirAll(storePath, 0o700))
			}
			if tt.blocked {
				require.NoError(t, os.WriteFile(cacheDir, []byte("not a directory"), 0o600))
			}
			configuredCacheDir := cacheDir
			if tt.disabled {
				configuredCacheDir = ""
			}
			streams, _, out, errOut := iostreams.NewTestIOStreams()
			if !tt.noLogger {
				streams = streams.WithLogger(slog.New(slog.NewJSONHandler(streams.ErrOut(), &slog.HandlerOptions{Level: tt.level})), nil)
			}
			source := &remoteSource{
				ref:     "registry.example.com/test/package:v1",
				arch:    "amd64",
				opts:    bundleinternal.ConfigOptions{CacheDir: configuredCacheDir},
				streams: streams,
			}
			remote, err := source.newZociRemote(t.Context())
			if tt.blocked {
				require.Error(t, err)
				assert.Nil(t, remote)
			} else {
				require.NoError(t, err)
				require.NotNil(t, remote)
			}
			assert.Empty(t, out.String(), "diagnostics must not reach stdout")
			if tt.noLogger || tt.level > slog.LevelDebug {
				assert.Empty(t, errOut.String())
				return
			}
			decoder := json.NewDecoder(strings.NewReader(errOut.String()))
			var event struct {
				Message string `json:"msg"`
				Level   string `json:"level"`
				Path    string `json:"path"`
				Reason  string `json:"reason"`
			}
			require.NoError(t, decoder.Decode(&event))
			assert.Equal(t, "DEBUG", event.Level)
			if tt.disabled {
				assert.Equal(t, "package OCI caching disabled", event.Message)
				assert.Equal(t, "no cache directory configured", event.Reason)
				assert.Empty(t, event.Path)
				assert.NoDirExists(t, cacheDir)
			} else {
				assert.Equal(t, "initializing package OCI cache", event.Message)
				assert.Equal(t, storePath, event.Path)
				if !tt.blocked {
					require.NoError(t, decoder.Decode(&event))
					assert.Equal(t, "package OCI cache initialized", event.Message)
					assert.Equal(t, storePath, event.Path)
					assert.DirExists(t, storePath)
				}
			}
			assert.False(t, decoder.More(), "unexpected extra diagnostics")
		})
	}
}
