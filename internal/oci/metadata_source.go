// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry"
)

// metadataReadSource buffers bounded metadata reads so cache errors returned
// while reading or closing the body can be retried before exposing any bytes.
// It is not a streaming source for package payloads or graph copies.
type metadataReadSource struct {
	oras.ReadOnlyTarget
	mu       sync.Mutex
	fetcher  content.Fetcher
	openErr  error
	cacheDir string
	open     func(context.Context) (oras.ReadOnlyTarget, error)
	streams  iostreams.IOStreams
}

var (
	_ oras.ReadOnlyTarget          = (*metadataReadSource)(nil)
	_ content.ReadOnlyGraphStorage = (*metadataReadSource)(nil)
	_ registry.ReferrerLister      = (*metadataReadSource)(nil)
)

// NewRemoteMetadataSource adds bounded cache fallback to remote metadata reads.
// One cache failure switches this source permanently to uncached reads. Tag
// resolution and signature discovery remain live; retries fetch the same digest.
func NewRemoteMetadataSource(ctx context.Context, ref string, opts bundleinternal.ConfigOptions, streams iostreams.IOStreams) (oras.ReadOnlyTarget, error) {
	return newRemoteMetadataSource(ctx, opts.CacheDir, streams, func(ctx context.Context, cacheDir string) (oras.ReadOnlyTarget, error) {
		attempt := opts
		attempt.CacheDir = cacheDir
		return NewRemoteReadSource(ctx, ref, attempt, streams)
	})
}

func newRemoteMetadataSource(ctx context.Context, cacheDir string, streams iostreams.IOStreams, open func(context.Context, string) (oras.ReadOnlyTarget, error)) (oras.ReadOnlyTarget, error) {
	source := &metadataReadSource{
		cacheDir: cacheDir,
		streams:  streams,
		open: func(ctx context.Context) (oras.ReadOnlyTarget, error) {
			return open(ctx, "")
		},
	}
	target, err := open(ctx, cacheDir)
	if err != nil {
		if ctx.Err() != nil || !IsCacheStorageError(err, cacheDir) {
			return nil, err
		}
		source.disableCache()
		var retryErr error
		target, retryErr = source.open(ctx)
		if retryErr != nil {
			return nil, errors.Join(err, fmt.Errorf("retrying metadata source without caching: %w", retryErr))
		}
	}
	source.ReadOnlyTarget = target
	source.fetcher = target
	return source, nil
}

// Fetch completes and verifies one metadata body before returning it. Parallel
// metadata reads share one transition to an uncached source; the mutex protects
// that transition, not downloads or the shared cache's upstream storage.
func (s *metadataReadSource) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	s.mu.Lock()
	fetcher, cacheDir, openErr := s.fetcher, s.cacheDir, s.openErr
	s.mu.Unlock()
	if openErr != nil {
		return nil, openErr
	}
	data, err := fetchMetadataBytes(ctx, fetcher, desc)
	if err == nil {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	if ctx.Err() != nil || !IsCacheStorageError(err, cacheDir) {
		return nil, err
	}
	s.mu.Lock()
	if s.cacheDir != "" {
		s.disableCache()
		target, retryErr := s.open(ctx)
		if retryErr != nil {
			s.openErr = errors.Join(err, fmt.Errorf("retrying metadata source without caching: %w", retryErr))
		} else {
			s.fetcher = target
		}
	}
	fetcher, openErr = s.fetcher, s.openErr
	s.mu.Unlock()
	if openErr != nil {
		return nil, openErr
	}
	data, retryErr := fetchMetadataBytes(ctx, fetcher, desc)
	if retryErr != nil {
		return nil, errors.Join(err, fmt.Errorf("retrying metadata read without caching: %w", retryErr))
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func fetchMetadataBytes(ctx context.Context, fetcher content.Fetcher, desc ocispec.Descriptor) ([]byte, error) {
	if err := desc.Digest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid metadata digest: %w", err)
	}
	if desc.Size < 0 {
		return nil, content.ErrInvalidDescriptorSize
	}
	if desc.Size > MaxFetchBytesSize {
		return nil, DescriptorTooLargeError{Digest: desc.Digest, Size: desc.Size, Limit: MaxFetchBytesSize}
	}
	r, err := fetcher.Fetch(ctx, desc)
	if err != nil {
		return nil, err
	}
	data, readErr := content.ReadAll(r, desc)
	// Upstream cache population can fail on Close, after the registry body
	// has been consumed. Do not discard that failure or expose partial bytes.
	if err := errors.Join(readErr, r.Close()); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *metadataReadSource) disableCache() {
	s.streams.Warn("metadata cache unavailable; retrying read once without caching", "cache", s.cacheDir)
	s.cacheDir = ""
}

// Predecessors retains registry-backed signature discovery, not cache indexes.
func (s *metadataReadSource) Predecessors(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	graph, ok := s.ReadOnlyTarget.(content.ReadOnlyGraphStorage)
	if !ok {
		return nil, fmt.Errorf("metadata source does not support signature discovery")
	}
	return graph.Predecessors(ctx, desc)
}

// Referrers retains native registry discovery when the remote supports it.
func (s *metadataReadSource) Referrers(ctx context.Context, desc ocispec.Descriptor, artifactType string, fn func([]ocispec.Descriptor) error) error {
	graph, ok := s.ReadOnlyTarget.(content.ReadOnlyGraphStorage)
	if !ok {
		return fmt.Errorf("metadata source does not support signature discovery")
	}
	refs, err := registry.Referrers(ctx, graph, desc, artifactType)
	if err != nil {
		return err
	}
	return fn(refs)
}
