// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/defenseunicorns/pkg/oci"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/ocischeme"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2/registry"
)

// Compile-time check: remoteSource must implement PackageSource.
var _ PackageSource = &remoteSource{}

// resolvePlainHTTP determines whether a registry reference should use plain HTTP.
func resolvePlainHTTP(ctx context.Context, ref string, opts ConfigOptions, transport http.RoundTripper) (bool, error) {
	if !opts.PlainHTTP {
		return false, nil
	}

	parsed, err := registry.ParseReference(ref)
	if err != nil {
		return false, fmt.Errorf("parsing OCI reference %q: %w", ref, err)
	}
	plainHTTP, err := ocischeme.From(ctx).UsePlainHTTP(ctx, parsed.Registry, ocischeme.ProbeOptions{
		InsecureSkipTLSVerify: opts.SkipTLSVerify,
		Transport:             transport,
	})
	if err != nil {
		return false, fmt.Errorf("determining registry transport for %q: %w", ref, err)
	}
	return plainHTTP, nil
}

// newZociRemote creates a zoci.Remote for ref using the configured platform and registry options.
func (s *remoteSource) newZociRemote(ctx context.Context, ref string) (*zoci.Remote, error) {
	platform := ocispec.Platform{
		Architecture: s.arch,
		OS:           oci.MultiOS,
	}
	plainHTTP, err := resolvePlainHTTP(ctx, ref, s.opts, nil)
	if err != nil {
		return nil, err
	}
	mods := []oci.Modifier{
		oci.WithPlainHTTP(plainHTTP),
		oci.WithInsecureSkipVerify(s.opts.SkipTLSVerify),
	}
	return zoci.NewRemote(ctx, ref, platform, mods...)
}

// resolveFilteredLayers connects to the remote registry, fetches the root
// manifest, and determines which layers are needed based on the filter.
// For Zarf packages it builds a minimal filtered layer list; for non-Zarf
// packages it returns all layers.
func (s *remoteSource) resolveFilteredLayers(ctx context.Context, filter filters.ComponentFilterStrategy) (*resolvedLayers, error) {
	remote, err := s.newZociRemote(ctx, s.ref)
	if err != nil {
		return nil, fmt.Errorf("creating OCI remote for %q: %w", s.ref, err)
	}

	rootDescriptor, err := remote.ResolveRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving root manifest for %q: %w", s.ref, err)
	}
	parsedRef, err := registry.ParseReference(s.ref)
	if err != nil {
		return nil, fmt.Errorf("parsing OCI reference %q: %w", s.ref, err)
	}
	parsedRef.Reference = rootDescriptor.Digest.String()
	pinnedRef := parsedRef.String()
	remote, err = s.newZociRemote(ctx, pinnedRef)
	if err != nil {
		return nil, fmt.Errorf("creating digest-pinned OCI remote for %q: %w", s.ref, err)
	}

	root, err := remote.FetchRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching root manifest %s for %q: %w", rootDescriptor.Digest, s.ref, err)
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
func (s *remoteSource) PullFiltered(ctx context.Context, tmpDir string, loadOptions layout.PackageLayoutOptions) (*layout.PackageLayout, error) {
	resolved, err := s.resolveFilteredLayers(ctx, loadOptions.Filter)
	if err != nil {
		return nil, err
	}

	if _, err := resolved.remote.PullPackage(ctx, tmpDir, s.concurrency(), resolved.layers...); err != nil {
		return nil, fmt.Errorf("pulling package %q: %w", s.ref, err)
	}

	loadOptions.IsPartial = resolved.isPartial
	pkgLayout, err := layout.LoadFromDir(ctx, tmpDir, loadOptions)
	if err != nil {
		return nil, fmt.Errorf("loading package layout for %q: %w", s.ref, err)
	}

	return pkgLayout, nil
}

// VerifyAndIngestFiltered resolves a remote package once, verifies the pulled
// layers, and ingests those exact layers into the bundle blob store.
func (s *remoteSource) VerifyAndIngestFiltered(ctx context.Context, tmpDir string, loadOptions layout.PackageLayoutOptions, blobDir string) ([]ociManifest, error) {
	resolved, err := s.resolveFilteredLayers(ctx, loadOptions.Filter)
	if err != nil {
		return nil, err
	}

	if _, err := resolved.remote.PullPackage(ctx, tmpDir, s.concurrency(), resolved.layers...); err != nil {
		return nil, fmt.Errorf("pulling package %q: %w", s.ref, err)
	}

	loadOptions.IsPartial = resolved.isPartial
	pkgLayout, err := layout.LoadFromDir(ctx, tmpDir, loadOptions)
	if err != nil {
		return nil, fmt.Errorf("loading package layout for %q: %w", s.ref, err)
	}
	defer func() {
		if err := pkgLayout.Cleanup(); err != nil {
			s.streams.Warn("failed to remove verified package layout", "path", pkgLayout.DirPath(), "error", err)
		}
	}()

	return s.ingestResolved(ctx, resolved, tmpDir, blobDir)
}

// IngestFiltered ingests a Zarf package from an OCI registry into the bundle's
// blob directory, applying the component filter BEFORE downloading.
// Returns manifest descriptors for the bundle's OCI index.
func (s *remoteSource) IngestFiltered(ctx context.Context, filter filters.ComponentFilterStrategy, blobDir string) ([]ociManifest, error) {
	resolved, err := s.resolveFilteredLayers(ctx, filter)
	if err != nil {
		return nil, err
	}
	return s.ingestResolved(ctx, resolved, "", blobDir)
}

// ingestResolved ingests a package from one resolved root. When stagedDir is
// non-empty, package layers are copied from the already-verified staged files
// instead of being fetched again.
func (s *remoteSource) ingestResolved(ctx context.Context, resolved *resolvedLayers, stagedDir, blobDir string) ([]ociManifest, error) {
	// Build set of needed digests from the filtered layer list
	neededDigests := make(map[string]bool, len(resolved.layers))
	for _, l := range resolved.layers {
		neededDigests[l.Digest.String()] = true
	}

	rawManifest, err := json.Marshal(resolved.root)
	if err != nil {
		return nil, fmt.Errorf("marshalling resolved manifest for %q: %w", s.ref, err)
	}

	var im ociImageManifest
	if err := json.Unmarshal(rawManifest, &im); err != nil {
		return nil, fmt.Errorf("parsing manifest for %q: %w", s.ref, err)
	}

	// Filter manifest layers to only those we need
	if resolved.isPartial {
		filtered := make([]ociDescriptor, 0, len(im.Layers))
		for _, l := range im.Layers {
			ld, err := udsoci.ParseDigest(l.Digest)
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
	configDesc, err := udsoci.DescriptorFromOCI(im.Config)
	if err != nil {
		return nil, err
	}
	configData, err := resolved.remote.FetchLayer(ctx, configDesc)
	if err != nil {
		return nil, fmt.Errorf("fetching config for %q: %w", s.ref, err)
	}
	configDigest, err := udsoci.ParseDigest(im.Config.Digest)
	if err != nil {
		return nil, err
	}
	if err := udsoci.WriteBlobBytesIfMissingAndVerify(blobDir, configDigest, configData); err != nil {
		return nil, fmt.Errorf("writing config blob: %w", err)
	}

	// Fetch and write layer blobs concurrently
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(s.concurrency())
	cleanStagedDir := ""
	if stagedDir != "" {
		cleanStagedDir, err = filepath.Abs(filepath.Clean(stagedDir))
		if err != nil {
			return nil, fmt.Errorf("resolving staged package directory: %w", err)
		}
	}
	for _, l := range im.Layers {
		g.Go(func() error {
			ld, err := udsoci.ParseDigest(l.Digest)
			if err != nil {
				return err
			}

			if stagedDir != "" {
				title := l.Annotations[zarfLayerTitleAnnotation]
				if title == "" {
					return fmt.Errorf("resolved package layer %s has no title annotation", l.Digest)
				}
				src, err := safeLayerDestinationPath(cleanStagedDir, stagedDir, title)
				if err != nil {
					return err
				}
				return udsoci.CopyBlobFileIfMissingAndVerify(blobDir, src, ld)
			}

			desc, err := udsoci.DescriptorFromOCI(l)
			if err != nil {
				return err
			}
			rc, err := resolved.remote.Repo().Fetch(gctx, desc)
			if err != nil {
				return fmt.Errorf("fetching layer %s: %w", l.Digest, err)
			}
			defer func() { _ = rc.Close() }()

			return udsoci.WriteBlobReaderIfMissingAndVerify(blobDir, ld, rc)
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
	manifestDigest, err := udsoci.WriteAndDigestBlob(blobDir, newManifestBytes)
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
