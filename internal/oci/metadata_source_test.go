// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
)

// TestMetadataCacheFallback checks the single fallback budget across opening,
// fetching, reading, and closing metadata. Every later read stays uncached.
func TestMetadataCacheFallback(t *testing.T) {
	for _, phase := range []string{"open", "fetch", "read", "close"} {
		for _, retryFails := range []bool{false, true} {
			t.Run(phase+map[bool]string{false: "/recovers", true: "/retry fails"}[retryFails], func(t *testing.T) {
				cacheDir := t.TempDir()
				cacheErr := &os.PathError{Op: "write", Path: filepath.Join(BundleCachePath(cacheDir), "images", "ingest"), Err: os.ErrPermission}
				retryErr := errors.New("registry unavailable")
				data := []byte("metadata")
				desc := content.NewDescriptorFromBytes("application/octet-stream", data)
				store := memory.New()
				require.NoError(t, store.Push(t.Context(), desc, bytes.NewReader(data)))
				cached := &metadataFetchTarget{Store: store, fetch: func(context.Context, ocispec.Descriptor) (io.ReadCloser, error) {
					switch phase {
					case "fetch":
						return nil, cacheErr
					case "read":
						return &metadataTestBody{Reader: iotestErrorReader{err: cacheErr}}, nil
					default:
						return &metadataTestBody{Reader: bytes.NewReader(data), closeErr: cacheErr}, nil
					}
				}}
				streams, _, _, errOut := iostreams.NewTestIOStreams()
				streams = streams.WithLogger(slog.New(slog.NewTextHandler(streams.ErrOut(), nil)), nil)
				var opened []string
				source, err := newRemoteMetadataSource(t.Context(), cacheDir, streams, func(_ context.Context, dir string) (oras.ReadOnlyTarget, error) {
					opened = append(opened, dir)
					if dir != "" {
						if phase == "open" {
							return nil, cacheErr
						}
						return cached, nil
					}
					if retryFails {
						return nil, retryErr
					}
					return store, nil
				})
				if err == nil {
					var got []byte
					got, err = FetchBytes(t.Context(), source, desc)
					if !retryFails {
						assert.Equal(t, data, got)
						// A second metadata read must not reopen the broken cache.
						_, nextErr := FetchBytes(t.Context(), source, desc)
						require.NoError(t, nextErr)
					} else {
						_, nextErr := FetchBytes(t.Context(), source, desc)
						require.ErrorIs(t, nextErr, retryErr)
					}
				}
				if retryFails {
					require.ErrorIs(t, err, cacheErr)
					require.ErrorIs(t, err, retryErr)
				} else {
					require.NoError(t, err)
				}
				assert.Equal(t, []string{cacheDir, ""}, opened)
				assert.Equal(t, 1, strings.Count(errOut.String(), "retrying read once without caching"))
			})
		}
	}
}

// TestMetadataCacheDoesNotRetryOtherFailures bounds fallback to cache filesystem
// failures, excluding registry errors, cancellation, and malformed cache indexes.
func TestMetadataCacheDoesNotRetryOtherFailures(t *testing.T) {
	for _, phase := range []string{"open", "fetch"} {
		for _, failure := range []error{errors.New("registry unavailable"), errors.New("invalid cache index JSON"), context.Canceled, context.DeadlineExceeded} {
			t.Run(phase+"/"+failure.Error(), func(t *testing.T) {
				streams, _, _, errOut := iostreams.NewTestIOStreams()
				streams = streams.WithLogger(slog.New(slog.NewTextHandler(streams.ErrOut(), nil)), nil)
				opens := 0
				source, err := newRemoteMetadataSource(t.Context(), t.TempDir(), streams, func(context.Context, string) (oras.ReadOnlyTarget, error) {
					opens++
					if phase == "open" {
						return nil, failure
					}
					return &metadataFetchTarget{Store: memory.New(), fetch: func(context.Context, ocispec.Descriptor) (io.ReadCloser, error) { return nil, failure }}, nil
				})
				if err == nil {
					_, err = FetchBytes(t.Context(), source, content.NewDescriptorFromBytes("application/octet-stream", []byte("metadata")))
				}
				require.ErrorIs(t, err, failure)
				assert.Equal(t, 1, opens)
				assert.NotContains(t, errOut.String(), "retrying")
			})
		}
	}
}

// TestMetadataCacheFallbackPreservesDescriptor simulates a moving tag during
// fallback: the retry must request the original descriptor, not resolve it again.
func TestMetadataCacheFallbackPreservesDescriptor(t *testing.T) {
	cacheDir := t.TempDir()
	store := memory.New()
	data := []byte("original metadata")
	desc, err := PushBytes(t.Context(), store, "application/octet-stream", data, nil)
	require.NoError(t, err)
	require.NoError(t, store.Tag(t.Context(), desc, "v1"))
	source, err := newRemoteMetadataSource(t.Context(), cacheDir, iostreams.IOStreams{}, func(ctx context.Context, dir string) (oras.ReadOnlyTarget, error) {
		if dir == "" {
			replacement, err := PushBytes(ctx, store, "application/octet-stream", []byte("replacement metadata"), nil)
			require.NoError(t, err)
			require.NoError(t, store.Tag(ctx, replacement, "v1"))
			return store, nil
		}
		return &metadataFetchTarget{Store: store, fetch: func(context.Context, ocispec.Descriptor) (io.ReadCloser, error) {
			return nil, &os.PathError{Op: "write", Path: BundleCachePath(cacheDir), Err: os.ErrPermission}
		}}, nil
	})
	require.NoError(t, err)
	resolved, err := source.Resolve(t.Context(), "v1")
	require.NoError(t, err)
	got, err := FetchBytes(t.Context(), source, resolved)
	require.NoError(t, err)
	assert.Equal(t, data, got)
	current, err := store.Resolve(t.Context(), "v1")
	require.NoError(t, err)
	assert.NotEqual(t, resolved.Digest, current.Digest)
}

// TestMetadataCacheFallbackSharesTransition checks adapter state, not upstream
// cache safety. Concurrent failed reads open one uncached source and each retry
// its own descriptor, without serializing the original metadata downloads.
func TestMetadataCacheFallbackSharesTransition(t *testing.T) {
	cacheDir := t.TempDir()
	store := memory.New()
	data := []byte("metadata")
	desc, err := PushBytes(t.Context(), store, "application/octet-stream", data, nil)
	require.NoError(t, err)
	const readers = 4
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	var entered atomic.Int64
	ready := make(chan struct{})
	var opens atomic.Int64
	source, err := newRemoteMetadataSource(ctx, cacheDir, iostreams.IOStreams{}, func(_ context.Context, stringDir string) (oras.ReadOnlyTarget, error) {
		opens.Add(1)
		if stringDir == "" {
			return store, nil
		}
		return &metadataFetchTarget{Store: store, fetch: func(ctx context.Context, _ ocispec.Descriptor) (io.ReadCloser, error) {
			if entered.Add(1) == readers {
				close(ready)
			}
			select {
			case <-ready:
				return nil, &os.PathError{Op: "write", Path: BundleCachePath(cacheDir), Err: os.ErrPermission}
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}}, nil
	})
	require.NoError(t, err)
	var wg sync.WaitGroup
	failures := make(chan error, readers)
	for range readers {
		wg.Go(func() {
			got, err := FetchBytes(ctx, source, desc)
			failures <- err
			assert.Equal(t, data, got)
		})
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	assert.EqualValues(t, 2, opens.Load())
}

// TestMetadataCacheFallbackLimits checks that disabled caching, cancellation,
// and invalid descriptor sizes cannot trigger fallback or unbounded buffering.
func TestMetadataCacheFallbackLimits(t *testing.T) {
	for _, scenario := range []string{"disabled", "canceled", "oversized", "negative size", "digest mismatch", "retry fetch fails"} {
		t.Run(scenario, func(t *testing.T) {
			cacheDir := t.TempDir()
			cacheErr := &os.PathError{Op: "write", Path: BundleCachePath(cacheDir), Err: os.ErrPermission}
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			data := []byte("metadata")
			desc := content.NewDescriptorFromBytes("application/octet-stream", data)
			switch scenario {
			case "disabled":
				cacheDir = ""
			case "oversized":
				desc.Size = MaxFetchBytesSize + 1
			case "negative size":
				desc.Size = -1
			}
			var opens, fetches int
			source, err := newRemoteMetadataSource(ctx, cacheDir, iostreams.IOStreams{}, func(context.Context, string) (oras.ReadOnlyTarget, error) {
				opens++
				return &metadataFetchTarget{Store: memory.New(), fetch: func(context.Context, ocispec.Descriptor) (io.ReadCloser, error) {
					fetches++
					if scenario == "canceled" {
						cancel()
					}
					if scenario == "digest mismatch" {
						return io.NopCloser(bytes.NewReader([]byte("bad data"))), nil
					}
					return nil, cacheErr
				}}, nil
			})
			require.NoError(t, err)
			_, err = FetchBytes(ctx, source, desc)
			require.Error(t, err)
			switch scenario {
			case "oversized", "negative size":
				assert.Zero(t, fetches)
				assert.Equal(t, 1, opens)
			case "retry fetch fails":
				assert.Equal(t, 2, opens)
				assert.Equal(t, 2, fetches, "no third fetch even when the uncached failure names the cache")
			default:
				assert.Equal(t, 1, opens)
				assert.Equal(t, 1, fetches)
			}
		})
	}
}

type metadataFetchTarget struct {
	*memory.Store
	fetch func(context.Context, ocispec.Descriptor) (io.ReadCloser, error)
}

func (s *metadataFetchTarget) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	return s.fetch(ctx, desc)
}

type metadataTestBody struct {
	io.Reader
	closeErr error
}

func (b *metadataTestBody) Close() error { return b.closeErr }

type iotestErrorReader struct{ err error }

func (r iotestErrorReader) Read([]byte) (int, error) { return 0, r.err }
