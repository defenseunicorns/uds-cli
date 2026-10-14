// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	oras "oras.land/oras-go/v2"
	oraci "oras.land/oras-go/v2/content/oci"
)

// TestPullCacheFallback checks the retry policy: a cache failure gets one uncached
// attempt, while cancellation, registry errors, and archive failures do not.
// If the retry also fails, both errors must remain available to the caller.
func TestPullCacheFallback(t *testing.T) {
	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)
	ref := "example.com/test/fallback:v1"
	pushArchTestBundle(t, store, ref, createArchTestBundle(t, "fallback", "1.0.0", runtime.GOARCH))
	for _, scenario := range []string{"recover", "retry fails", "registry failure", "canceled", "disabled", "archive failure"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := newTestConfig()
			cfg.Options.CacheDir = t.TempDir()
			cfg.Options.TmpDir = t.TempDir()
			cacheDir := cfg.Options.CacheDir
			cacheErr := &os.PathError{Op: "mkdir", Path: filepath.Join(cacheDir, "bundle-oci", "images"), Err: os.ErrPermission}
			registryErr := errors.New("registry unavailable")
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			if scenario == "disabled" {
				cfg.Options.CacheDir = ""
			}
			var attempts []string
			archives := 0
			result, err := NewDefaultPuller().PullBundle(ctx, ref, t.TempDir(), PullOptions{
				Config: cfg, SkipSignatureVerification: true,
				PullHooks: PullHooks{
					ToOrasTarget: func(_ context.Context, _ string, opts *PullOptions) (oras.Target, error) {
						attempts = append(attempts, opts.Config.Options.CacheDir)
						entries, readErr := os.ReadDir(cfg.Options.TmpDir)
						require.NoError(t, readErr)
						assert.Len(t, entries, 1, "the failed attempt must be cleaned before retrying")
						if scenario == "archive failure" {
							return store, nil
						}
						if len(attempts) == 1 {
							if scenario == "registry failure" {
								return nil, registryErr
							}
							if scenario == "canceled" {
								cancel()
							}
							return nil, cacheErr
						}
						if scenario == "retry fails" {
							return nil, errors.Join(cacheErr, registryErr)
						}
						return store, nil
					},
					CreateBundleArchive: func(context.Context, iostreams.IOStreams, string, string, ocispec.Index, string) (string, error) {
						archives++
						if scenario == "archive failure" {
							// Even a path inside the cache is not a cache read error
							// when it originates from output archive creation.
							return "", cacheErr
						}
						return "bundle.tar.zst", nil
					},
				},
			})
			if scenario == "recover" {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, []string{cacheDir, ""}, attempts)
				assert.Equal(t, 1, archives)
			} else {
				require.Error(t, err)
				assert.Nil(t, result)
				if scenario == "retry fails" {
					assert.Len(t, attempts, 2, "a failed uncached retry must not trigger another attempt")
					require.ErrorIs(t, err, cacheErr)
					require.ErrorIs(t, err, registryErr)
				} else {
					assert.Len(t, attempts, 1)
				}
			}
			require.NotEmpty(t, attempts)
			assert.Equal(t, attempts[0], cfg.Options.CacheDir, "fallback must not change the caller's configuration")
			entries, readErr := os.ReadDir(cfg.Options.TmpDir)
			require.NoError(t, readErr)
			assert.Empty(t, entries)
		})
	}
}

type failingPullFetchTarget struct {
	*oraci.Store
	fetch func(context.Context, ocispec.Descriptor) (io.ReadCloser, error)
}

func (s failingPullFetchTarget) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	return s.fetch(ctx, desc)
}

// TestPullCacheFallbackPreservesResolvedDigest checks that an uncached retry
// retrieves the originally selected bundle even if its registry tag moves.
// Both attempts must verify that same bundle, and the failed workspace is removed.
func TestPullCacheFallbackPreservesResolvedDigest(t *testing.T) {
	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)
	ref := "example.com/test/moving:v1"
	pushArchTestBundle(t, store, ref, createArchTestBundle(t, "original", "1.0.0", runtime.GOARCH))
	child, indexBytes, err := ResolveBundleChild(t.Context(), store, "v1", runtime.GOARCH)
	require.NoError(t, err)
	evidence := []byte("expected signature evidence")
	pushSignatureEvidence(t, store, child, evidence)
	var original ocispec.Index
	require.NoError(t, json.Unmarshal(indexBytes, &original))
	pushArchTestBundle(t, store, "example.com/test/moving:v2", createArchTestBundle(t, "replacement", "2.0.0", runtime.GOARCH))
	replacement, err := store.Resolve(t.Context(), "v2")
	require.NoError(t, err)
	cfg := newTestConfig()
	cfg.Options.CacheDir = t.TempDir()
	cfg.Options.TmpDir = t.TempDir()
	cacheErr := &os.PathError{Op: "write", Path: filepath.Join(cfg.Options.CacheDir, "bundle-oci", "images", "ingest"), Err: os.ErrPermission}
	attempts := 0
	verifications := 0
	result, err := NewDefaultPuller().PullBundle(t.Context(), ref, t.TempDir(), PullOptions{
		Config: cfg,
		PullHooks: PullHooks{
			ToOrasTarget: func(context.Context, string, *PullOptions) (oras.Target, error) {
				attempts++
				if attempts == 2 {
					entries, err := os.ReadDir(cfg.Options.TmpDir)
					require.NoError(t, err)
					assert.Len(t, entries, 1, "the partial graph-copy workspace must be gone before retrying")
					// Move the tag between attempts; the retry must use the
					// already selected child, not resolve this tag again.
					require.NoError(t, store.Tag(t.Context(), replacement, "v1"))
					return store, nil
				}
				return failingPullFetchTarget{Store: store, fetch: func(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
					if desc.Digest == original.Manifests[0].Digest {
						return nil, cacheErr
					}
					return store.Fetch(ctx, desc)
				}}, nil
			},
			VerifyBundle: func(_ context.Context, index, signature []byte) error {
				verifications++
				assert.Equal(t, indexBytes, index)
				assert.Equal(t, evidence, signature)
				return nil
			},
			CreateBundleArchive: func(_ context.Context, _ iostreams.IOStreams, _ string, _ string, idx ocispec.Index, _ string) (string, error) {
				assert.Equal(t, original, idx)
				return "bundle.tar.zst", nil
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ref, result.OCIReference)
	assert.Equal(t, 2, attempts)
	assert.Equal(t, 2, verifications, "the retry must verify the same bundle again")
}
