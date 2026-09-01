// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/oci"
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	"oras.land/oras-go/v2/content"
)

// PackageSource abstracts local and OCI package retrieval.
type PackageSource interface {
	// PullFiltered retrieves a deployable layout using the supplied filter.
	PullFiltered(context.Context, string, layout.PackageLayoutOptions) (*layout.PackageLayout, error)
	// IngestFiltered copies filtered package content into an OCI store.
	IngestFiltered(context.Context, filters.ComponentFilterStrategy, *udsoci.Store) ([]ocispec.Descriptor, error)
	// VerifyAndIngestFiltered verifies the retrieved package before ingestion.
	VerifyAndIngestFiltered(context.Context, string, layout.PackageLayoutOptions, *udsoci.Store) ([]ocispec.Descriptor, error)
}
type localSource struct {
	path      string
	arch      string
	bundleDir string
	tmpDir    string
	streams   iostreams.IOStreams
}
type remoteSource struct {
	ref     string
	arch    string
	opts    bundleinternal.ConfigOptions
	streams iostreams.IOStreams
}
type resolvedLayers struct {
	remote    *zoci.Remote
	root      *oci.Manifest
	layers    []ocispec.Descriptor
	isPartial bool
}
type layerIdentity struct {
	digest string
	title  string
}

// NewPackageSource returns a PackageSource for the given source string.
// OCI references use zoci.NewRemote; everything else is treated as a local
// path resolved against bundleDir.
// streams carries the leveled logger used for ingest/pull diagnostics.
func NewPackageSource(source string, opts bundleinternal.ConfigOptions, bundleDir string, streams iostreams.IOStreams) PackageSource {
	if udsoci.IsOCIReference(source) {
		return &remoteSource{
			ref:     udsoci.TrimScheme(source),
			arch:    opts.Architecture,
			opts:    opts,
			streams: streams,
		}
	}
	return &localSource{
		path:      source,
		arch:      opts.Architecture,
		bundleDir: bundleDir,
		tmpDir:    opts.TmpDir,
		streams:   streams,
	}
}

// isZarfPackage checks if the directory is a Zarf package by looking for zarf.yaml
func isZarfPackage(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "zarf.yaml"))
	return err == nil
}

// selectZarfLayers asks Zarf to select the complete package graph for the
// requested components before any package content is copied.
func selectZarfLayers(ctx context.Context, root *oci.Manifest, fetcher content.Fetcher, filter filters.ComponentFilterStrategy) ([]ocispec.Descriptor, bool, error) {
	pkg, err := zoci.FetchZarfYAML(ctx, root, fetcher)
	if err != nil {
		return nil, false, fmt.Errorf("fetching zarf.yaml: %w: %w", ErrFetchPackageMetadata, err)
	}
	filteredPackage, err := filters.Apply(api.NewPackageDefinitionFromV1alpha1(pkg), filter)
	if err != nil {
		return nil, false, fmt.Errorf("%w for package %q: %w", ErrApplyComponentFilter, pkg.Metadata.Name, err)
	}
	components := filteredPackage.AsV1alpha1().Components
	layers, err := zoci.AssembleLayers(ctx, root, fetcher, components)
	if err != nil {
		return nil, false, fmt.Errorf("%w for package %q: %w", ErrAssemblePackageLayers, pkg.Metadata.Name, err)
	}
	return layers, len(components) < len(pkg.Components), nil
}

// BuildComponentFilter creates a component filter strategy from optional component names.
// When optionalComponents is empty, only Required and Default Zarf components are included.
// When optionalComponents lists component names, those are explicitly included alongside
// Required components. Use the "-name" prefix to explicitly exclude a component.
func BuildComponentFilter(optionalComponents []string) filters.ComponentFilterStrategy {
	return filters.Combine(
		filters.ForDeploy(strings.Join(optionalComponents, ","), false),
	)
}
