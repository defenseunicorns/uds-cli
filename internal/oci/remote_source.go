// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	zarfoci "github.com/zarf-dev/zarf/src/pkg/oci"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry"
)

// remoteReadSource preserves registry discovery alongside upstream cache-backed
// reads. Embedding only read interfaces prevents accidental publication through
// this adapter; cache writes remain an implementation detail of upstream Fetch.
type remoteReadSource struct {
	oras.ReadOnlyTarget
	remote *zarfoci.OrasRemote
}

var (
	_ oras.ReadOnlyTarget          = (*remoteReadSource)(nil)
	_ content.ReadOnlyGraphStorage = (*remoteReadSource)(nil)
	_ registry.ReferrerLister      = (*remoteReadSource)(nil)
)

// NewRemoteReadSource constructs a registry source using upstream authentication
// and caching. An empty CacheDir disables caching. Callers must retain this
// source for content reads instead of fetching through an underlying repository.
func NewRemoteReadSource(ctx context.Context, ref string, opts bundleinternal.ConfigOptions, streams iostreams.IOStreams) (oras.ReadOnlyTarget, error) {
	plainHTTP, err := ResolvePlainHTTP(ctx, TrimScheme(ref), opts, nil)
	if err != nil {
		return nil, err
	}
	log := streams.Logger()
	if log == nil {
		// Upstream defaults to the process-global logger unless explicitly
		// supplied one. Library diagnostics must stay with the caller's streams.
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	modifiers := []zarfoci.Modifier{
		zarfoci.WithPlainHTTP(plainHTTP),
		zarfoci.WithInsecureSkipVerify(opts.SkipTLSVerify),
		zarfoci.WithLogger(log),
	}
	cachePath := BundleCachePath(opts.CacheDir)
	if cachePath == "" {
		streams.Debug("bundle OCI caching disabled", "reason", "no cache directory configured")
	} else {
		// Match the images/ store opened by Zarf's package remote so create
		// and bundle reads can reuse identical content-addressed blobs.
		storePath := filepath.Join(cachePath, zoci.ImageCacheDirectory)
		streams.Debug("initializing bundle OCI cache", "path", storePath)
		// Let upstream construct the store as well as manage its bytes. Package
		// and bundle reads then inherit upstream cache fixes together.
		modifier, err := zoci.GetOCICacheModifier(ctx, cachePath)
		if err != nil {
			return nil, fmt.Errorf("opening bundle OCI cache %q: %w", storePath, err)
		}
		modifiers = append(modifiers, modifier)
		streams.Debug("bundle OCI cache initialized", "path", storePath)
	}
	remote, err := zarfoci.NewOrasRemote(TrimScheme(ref), zarfoci.PlatformForArch(opts.Architecture), modifiers...)
	if err != nil {
		return nil, fmt.Errorf("creating registry read source: %w", err)
	}
	repo := remote.Repo()
	return &remoteReadSource{ReadOnlyTarget: repo, remote: remote}, nil
}

// Fetch retains upstream caching rather than reading through Repo directly.
func (s *remoteReadSource) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	return s.remote.Fetch(ctx, desc)
}

// FetchReference resolves live, then fetches the selected immutable bytes through
// upstream caching. Do not promote the repository's cache-bypassing method.
func (s *remoteReadSource) FetchReference(ctx context.Context, reference string) (ocispec.Descriptor, io.ReadCloser, error) {
	desc, err := s.Resolve(ctx, reference)
	if err != nil {
		return ocispec.Descriptor{}, nil, err
	}
	r, err := s.Fetch(ctx, desc)
	return desc, r, err
}

// Predecessors discovers referencing artifacts in the registry, not the cache.
// Cached blobs cannot tell us whether signature evidence has changed remotely.
func (s *remoteReadSource) Predecessors(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	return s.remote.Repo().Predecessors(ctx, desc)
}

// Referrers preserves the registry's native signature-discovery capability.
func (s *remoteReadSource) Referrers(ctx context.Context, desc ocispec.Descriptor, artifactType string, fn func([]ocispec.Descriptor) error) error {
	return s.remote.Repo().Referrers(ctx, desc, artifactType, fn)
}
