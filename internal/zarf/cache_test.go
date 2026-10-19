// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/google/go-containerregistry/pkg/registry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
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

func TestRemovePackageUsesConfiguredCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	var denyBlobs atomic.Bool
	handler := registry.New()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if denyBlobs.Load() && r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") {
			http.Error(w, "blob downloads disabled", http.StatusForbidden)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	// The unchanged upstream removal loader uses HTTPS. Trust only this test's
	// local registry without changing the removal transport options.
	transport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = transport })
	ref, pkgLayout := pushCacheTestPackage(t, server.URL)
	cacheDir := t.TempDir()
	opts := defaultPkgOpts()
	opts.Config.Options.Architecture = runtime.GOARCH
	opts.Config.Options.Concurrency = 1
	opts.Config.Options.CacheDir = cacheDir
	pkg := &spec.Package{Name: "bundle-package", Source: "oci://" + ref}
	remove := func() error {
		// A populated, empty deployment snapshot stops removal after the real
		// metadata loader runs. No cluster requests or removal actions occur.
		remover := &ZarfRemover{cluster: &cluster.Cluster{}, deployedLoaded: true}
		return remover.RemovePackage(t.Context(), pkg, opts)
	}
	require.ErrorIs(t, remove(), ErrPackageNotDeployed)
	require.DirExists(t, filepath.Join(cacheDir, "bundle-oci", "images", "blobs"))
	denyBlobs.Store(true)
	require.ErrorIs(t, remove(), ErrPackageNotDeployed, "fresh removal must reuse cached metadata")

	// Publish the same package without checksum metadata. Cached zarf.yaml is
	// not enough: the original upstream loader must still reject this package.
	root, err := pkgLayout.Manifest()
	require.NoError(t, err)
	malformed := root.Manifest
	malformed.Layers = nil
	for _, layer := range root.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] != layout.Checksums {
			malformed.Layers = append(malformed.Layers, layer)
		}
	}
	data, err := json.Marshal(malformed)
	require.NoError(t, err)
	remote, err := zoci.NewRemoteWithOptions(t.Context(), ref, ocispec.Platform{Architecture: runtime.GOARCH, OS: oci.MultiOS}, zoci.RemoteClientOptions{})
	require.NoError(t, err)
	desc := udsoci.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, data)
	require.NoError(t, udsoci.PushReferenceBytes(t.Context(), remote.Repo(), desc, data, "1.0.0"))
	denyBlobs.Store(false)
	err = remove()
	require.ErrorIs(t, err, ErrLoadPackage)
	require.ErrorContains(t, err, layout.Checksums)
}

func pushCacheTestPackage(t *testing.T, serverURL string) (string, *layout.PackageLayout) {
	t.Helper()
	pkgDir := t.TempDir()
	writeFilteredPackageFiles(t, pkgDir)
	pkgLayout, err := layout.LoadFromDir(t.Context(), pkgDir, layout.PackageLayoutOptions{VerificationStrategy: layout.VerifyNever})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
	ref := strings.TrimPrefix(strings.TrimPrefix(serverURL, "http://"), "https://") + "/cache/package:1.0.0"
	remote, err := zoci.NewRemoteWithOptions(t.Context(), ref, ocispec.Platform{Architecture: runtime.GOARCH, OS: oci.MultiOS}, zoci.RemoteClientOptions{RemoteOptions: zarfTypes.RemoteOptions{PlainHTTP: !strings.HasPrefix(serverURL, "https://")}})
	require.NoError(t, err)
	_, err = remote.PushPackage(t.Context(), pkgLayout, zoci.PublishOptions{OCIConcurrency: 1})
	require.NoError(t, err)
	return ref, pkgLayout
}
