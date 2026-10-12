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
	"github.com/zarf-dev/zarf/src/pkg/oci/cache"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	orasoci "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry"
	orasremote "oras.land/oras-go/v2/registry/remote"
)

// remoteReadSource preserves registry discovery alongside upstream cache-backed
// reads. Embedding only read interfaces prevents accidental publication through
// this adapter; cache writes remain an implementation detail of upstream Fetch.
type remoteReadSource struct {
	oras.ReadOnlyTarget
	registry.ReferenceFetcher
	repo *orasremote.Repository
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
	remote, err := zarfoci.NewOrasRemote(TrimScheme(ref), zarfoci.PlatformForArch(opts.Architecture),
		zarfoci.WithPlainHTTP(plainHTTP),
		zarfoci.WithInsecureSkipVerify(opts.SkipTLSVerify),
		zarfoci.WithLogger(log),
	)
	if err != nil {
		return nil, fmt.Errorf("creating registry read source: %w", err)
	}
	repo := remote.Repo()
	var source oras.ReadOnlyTarget = repo
	cachePath := BundleCachePath(opts.CacheDir)
	if cachePath == "" {
		streams.Debug("bundle OCI caching disabled", "reason", "no cache directory configured")
	} else {
		// Match the images/ store opened by Zarf's package remote so create
		// and bundle reads can reuse identical content-addressed blobs.
		storePath := filepath.Join(cachePath, zoci.ImageCacheDirectory)
		streams.Debug("initializing bundle OCI cache", "path", storePath)
		store, err := orasoci.NewWithContext(ctx, storePath)
		if err != nil {
			return nil, fmt.Errorf("opening bundle OCI cache %q: %w", storePath, err)
		}
		// Use the same upstream wrapper as OrasRemote's cached Fetch path.
		// It owns population and reuse; UDS adds no blob publication algorithm.
		source = cache.New(repo, store)
		streams.Debug("bundle OCI cache initialized", "path", storePath)
	}
	// Both the repository and its upstream cache wrapper implement reference
	// fetching. Preserve it so generic copy helpers do not bypass the wrapper.
	referenceFetcher, ok := source.(registry.ReferenceFetcher)
	if !ok {
		return nil, fmt.Errorf("registry read source does not support reference fetching")
	}
	return &remoteReadSource{
		ReadOnlyTarget:   source,
		ReferenceFetcher: referenceFetcher,
		repo:             repo,
	}, nil
}

// Predecessors discovers referencing artifacts in the registry, not the cache.
// Cached blobs cannot tell us whether signature evidence has changed remotely.
func (s *remoteReadSource) Predecessors(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	return s.repo.Predecessors(ctx, desc)
}

// Referrers preserves the registry's native signature-discovery capability.
func (s *remoteReadSource) Referrers(ctx context.Context, desc ocispec.Descriptor, artifactType string, fn func([]ocispec.Descriptor) error) error {
	return s.repo.Referrers(ctx, desc, artifactType, fn)
}
