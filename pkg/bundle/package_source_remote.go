// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	"golang.org/x/sync/errgroup"
)

// remoteSource pulls Zarf packages from OCI registries using zoci.NewRemote.
type remoteSource struct {
	ref     string
	arch    string
	opts    ConfigOptions
	streams iostreams.IOStreams
}

// Compile-time check: remoteSource must implement PackageSource.
var _ PackageSource = &remoteSource{}

// newZociRemote creates a zoci.Remote for the configured reference and registry options.
func (s *remoteSource) newZociRemote(ctx context.Context) (*zoci.Remote, error) {
	platform := ocispec.Platform{
		Architecture: s.arch,
		OS:           oci.MultiOS,
	}
	plainHTTP, err := resolvePlainHTTP(ctx, s.ref, s.opts, nil)
	if err != nil {
		return nil, err
	}
	mods := []oci.Modifier{
		oci.WithPlainHTTP(plainHTTP),
		oci.WithInsecureSkipVerify(s.opts.SkipTLSVerify),
	}
	return zoci.NewRemote(ctx, s.ref, platform, mods...)
}

// resolvedLayers holds the result of connecting to a remote and resolving
// which layers to fetch based on component filtering.
type resolvedLayers struct {
	remote    *zoci.Remote
	root      *oci.Manifest
	layers    []ocispec.Descriptor
	isPartial bool
}

// resolveFilteredLayers connects to the remote registry, fetches the root
// manifest, and determines which layers are needed based on the filter.
// For Zarf packages it builds a minimal filtered layer list; for non-Zarf
// packages it returns all layers.
func (s *remoteSource) resolveFilteredLayers(ctx context.Context, filter filters.ComponentFilterStrategy) (*resolvedLayers, error) {
	remote, err := s.newZociRemote(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating OCI remote for %q: %w", s.ref, err)
	}

	root, err := remote.FetchRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching root manifest for %q: %w", s.ref, err)
	}

	var layers []ocispec.Descriptor
	isPartial := false

	if isZarfOCIPackage(root) {
		pkg, err := remote.FetchZarfYAML(ctx)
		if err != nil {
			return nil, fmt.Errorf("fetching zarf.yaml from %q: %w", s.ref, err)
		}

		totalComponents := len(pkg.Components)
		filteredComponents, err := filter.Apply(pkg)
		if err != nil {
			return nil, fmt.Errorf("applying component filter for %q: %w", s.ref, err)
		}
		pkg.Components = filteredComponents

		layers, err = buildFilteredLayerList(ctx, remote, root, pkg)
		if err != nil {
			return nil, fmt.Errorf("building filtered layer list for %q: %w", s.ref, err)
		}

		isPartial = len(filteredComponents) < totalComponents
		s.streams.Debug("resolved filtered layers", "ref", s.ref, "totalComponents", totalComponents, "filteredComponents", len(filteredComponents), "partial", isPartial)
	} else {
		s.streams.Debug("non-Zarf package, using all layers", "ref", s.ref)
		layers = root.Layers
	}

	return &resolvedLayers{
		remote:    remote,
		root:      root,
		layers:    layers,
		isPartial: isPartial,
	}, nil
}

// concurrency returns the configured concurrency or the zoci default.
func (s *remoteSource) concurrency() int {
	if s.opts.Concurrency > 0 {
		return s.opts.Concurrency
	}
	return zoci.DefaultConcurrency
}

// PullFiltered pulls a Zarf package from an OCI registry, applying the component
// filter BEFORE downloading to avoid pulling unnecessary layers.
func (s *remoteSource) PullFiltered(ctx context.Context, filter filters.ComponentFilterStrategy, tmpDir string) (*layout.PackageLayout, error) {
	resolved, err := s.resolveFilteredLayers(ctx, filter)
	if err != nil {
		return nil, err
	}

	if _, err := resolved.remote.PullPackage(ctx, tmpDir, s.concurrency(), resolved.layers...); err != nil {
		return nil, fmt.Errorf("pulling package %q: %w", s.ref, err)
	}

	pkgLayout, err := layout.LoadFromDir(ctx, tmpDir, layout.PackageLayoutOptions{
		Filter:    filter,
		IsPartial: resolved.isPartial,
	})
	if err != nil {
		return nil, fmt.Errorf("loading package layout for %q: %w", s.ref, err)
	}

	return pkgLayout, nil
}

// IngestFiltered ingests a Zarf package from an OCI registry into the bundle's
// blob directory, applying the component filter BEFORE downloading.
// Returns manifest descriptors for the bundle's OCI index.
func (s *remoteSource) IngestFiltered(ctx context.Context, filter filters.ComponentFilterStrategy, blobDir string) ([]ociManifest, error) {
	resolved, err := s.resolveFilteredLayers(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Build set of needed digests from the filtered layer list
	neededDigests := make(map[string]bool, len(resolved.layers))
	for _, l := range resolved.layers {
		neededDigests[l.Digest.String()] = true
	}

	// Fetch the raw root manifest to get the internal structure
	rootDesc, err := resolved.remote.ResolveRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving root for %q: %w", s.ref, err)
	}
	rawManifest, err := resolved.remote.FetchLayer(ctx, rootDesc)
	if err != nil {
		return nil, fmt.Errorf("fetching manifest for %q: %w", s.ref, err)
	}

	var im ociImageManifest
	if err := json.Unmarshal(rawManifest, &im); err != nil {
		return nil, fmt.Errorf("parsing manifest for %q: %w", s.ref, err)
	}

	// Filter manifest layers to only those we need
	if resolved.isPartial {
		filtered := make([]ociDescriptor, 0, len(im.Layers))
		for _, l := range im.Layers {
			ld, err := parseDigest(l.Digest)
			if err != nil {
				return nil, fmt.Errorf("invalid layer digest in manifest for %q: %w", s.ref, err)
			}
			if neededDigests[ld.String()] {
				filtered = append(filtered, l)
			}
		}
		im.Layers = filtered
	}

	// Write config blob
	configDesc, err := descriptorFromOCI(im.Config)
	if err != nil {
		return nil, err
	}
	configData, err := resolved.remote.FetchLayer(ctx, configDesc)
	if err != nil {
		return nil, fmt.Errorf("fetching config for %q: %w", s.ref, err)
	}
	configDigest, err := parseDigest(im.Config.Digest)
	if err != nil {
		return nil, err
	}
	if err := writeBlobBytesIfMissingAndVerify(blobDir, configDigest, configData); err != nil {
		return nil, fmt.Errorf("writing config blob: %w", err)
	}

	// Fetch and write layer blobs concurrently
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(s.concurrency())
	for _, l := range im.Layers {
		g.Go(func() error {
			desc, err := descriptorFromOCI(l)
			if err != nil {
				return err
			}
			rc, err := resolved.remote.Repo().Fetch(gctx, desc)
			if err != nil {
				return fmt.Errorf("fetching layer %s: %w", l.Digest, err)
			}
			defer func() { _ = rc.Close() }()

			ld, err := parseDigest(l.Digest)
			if err != nil {
				return err
			}
			return writeBlobReaderIfMissingAndVerify(blobDir, ld, rc)
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Marshal the (possibly filtered) manifest, write as blob
	newManifestBytes, err := json.Marshal(im)
	if err != nil {
		return nil, fmt.Errorf("marshalling manifest: %w", err)
	}
	manifestDigest, err := writeAndDigestBlob(blobDir, newManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("writing manifest blob: %w", err)
	}

	mediaType := im.MediaType
	if mediaType == "" {
		mediaType = ocispec.MediaTypeImageManifest
	}

	return []ociManifest{{
		MediaType: mediaType,
		Digest:    manifestDigest.String(),
		Size:      int64(len(newManifestBytes)),
	}}, nil
}
