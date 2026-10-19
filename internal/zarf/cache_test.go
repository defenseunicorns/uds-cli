// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/google/go-containerregistry/pkg/registry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/oci"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
)

func TestPackageMetadataCache(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "configured"
		}
		t.Run(name, func(t *testing.T) {
			var downloads atomic.Int64
			var denyBlobs atomic.Bool
			handler := registry.New()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") {
					downloads.Add(1)
					if denyBlobs.Load() {
						http.Error(w, "blob reads disabled", http.StatusForbidden)
						return
					}
				}
				handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			ref, _ := pushCacheTestPackage(t, server.URL)
			cacheDir := filepath.Join(t.TempDir(), "cache")
			opts := bundleinternal.ConfigOptions{Architecture: runtime.GOARCH, PlainHTTP: true, Concurrency: 1}
			if enabled {
				opts.CacheDir = cacheDir
			}
			load := func() error {
				source := NewPackageSource("oci://"+ref, opts, "", iostreams.IOStreams{})
				got, err := source.LoadPackageSpec(t.Context(), filters.Empty())
				if err == nil {
					assert.Equal(t, "filtered", got.Name)
					assert.Equal(t, []string{"included", "excluded"}, got.Components)
				}
				return err
			}
			downloads.Store(0)
			require.NoError(t, load())
			assert.EqualValues(t, 1, downloads.Load(), "metadata reads fetch only zarf.yaml")
			if enabled {
				require.DirExists(t, filepath.Join(cacheDir, "bundle-oci", "images", "blobs", "sha256"))
				assert.NoDirExists(t, filepath.Join(cacheDir, "images"))
				assert.NoDirExists(t, filepath.Join(cacheDir, "layers"))
				denyBlobs.Store(true)
			}
			require.NoError(t, load(), "fresh sources must reuse upstream cached bytes")
			if enabled {
				assert.EqualValues(t, 1, downloads.Load())
			} else {
				assert.EqualValues(t, 2, downloads.Load(), "empty cache must not use Zarf's default cache")
				assert.NoDirExists(t, cacheDir)
			}
		})
	}
}

func TestRemoteSourcePullFilteredCache(t *testing.T) {
	var downloads atomic.Int64
	var denyBlobs atomic.Bool
	handler := registry.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") {
			downloads.Add(1)
			if denyBlobs.Load() {
				http.Error(w, "blob reads disabled", http.StatusForbidden)
				return
			}
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	ref, _ := pushCacheTestPackage(t, server.URL)
	opts := bundleinternal.ConfigOptions{
		Architecture: runtime.GOARCH,
		PlainHTTP:    true,
		CacheDir:     t.TempDir(),
		Concurrency:  1,
	}
	pull := func() {
		source := NewPackageSource(ref, opts, "", iostreams.IOStreams{})
		pkgLayout, err := source.PullFiltered(t.Context(), t.TempDir(), layout.PackageLayoutOptions{
			Filter:               filters.ForDeploy("included", false),
			VerificationStrategy: layout.VerifyNever,
		})
		require.NoError(t, err)
		assert.Equal(t, "filtered", pkgLayout.Definition().Metadata.Name)
		require.NoError(t, pkgLayout.Cleanup())
	}
	pull()
	coldDownloads := downloads.Load()
	require.Positive(t, coldDownloads)
	denyBlobs.Store(true)
	pull()
	assert.Equal(t, coldDownloads, downloads.Load(), "package pulls must use upstream cached bytes")
}

func TestLocalPackageSourceDoesNotOpenCache(t *testing.T) {
	pkgDir := t.TempDir()
	writeFilteredPackageFiles(t, pkgDir)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	// A file at the configured cache path would prevent remote cache creation.
	require.NoError(t, os.WriteFile(cacheDir, []byte("not a directory"), 0o600))
	source := NewPackageSource(pkgDir, bundleinternal.ConfigOptions{CacheDir: cacheDir}, "", iostreams.IOStreams{})
	got, err := source.LoadPackageSpec(t.Context(), filters.Empty())
	require.NoError(t, err)
	assert.Equal(t, "filtered", got.Name)
	data, err := os.ReadFile(cacheDir)
	require.NoError(t, err)
	assert.Equal(t, "not a directory", string(data))
}

func pushCacheTestPackage(t *testing.T, serverURL string) (string, *layout.PackageLayout) {
	t.Helper()
	pkgDir := t.TempDir()
	writeFilteredPackageFiles(t, pkgDir)
	pkgLayout, err := layout.LoadFromDir(t.Context(), pkgDir, layout.PackageLayoutOptions{VerificationStrategy: layout.VerifyNever})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
	ref := strings.TrimPrefix(serverURL, "http://") + "/cache/package:1.0.0"
	remote, err := zoci.NewRemoteWithOptions(t.Context(), ref, ocispec.Platform{Architecture: runtime.GOARCH, OS: oci.MultiOS}, zoci.RemoteClientOptions{RemoteOptions: zarfTypes.RemoteOptions{PlainHTTP: true}})
	require.NoError(t, err)
	_, err = remote.PushPackage(t.Context(), pkgLayout, zoci.PublishOptions{OCIConcurrency: 1})
	require.NoError(t, err)
	return ref, pkgLayout
}
