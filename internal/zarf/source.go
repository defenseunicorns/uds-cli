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
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
)

// NewPackageSource returns a PackageSource for the given source string.
// OCI references (detected by IsOCIReference) use zoci.NewRemote;
// everything else is treated as a local path resolved against bundleDir.
// streams carries the leveled logger used for ingest/pull diagnostics.
func NewPackageSource(source string, opts ConfigOptions, bundleDir string, streams iostreams.IOStreams) PackageSource {
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
func selectZarfLayers(ctx context.Context, root *oci.Manifest, fetcher udsoci.Fetcher, filter filters.ComponentFilterStrategy) ([]ocispec.Descriptor, bool, error) {
	pkg, err := zoci.FetchZarfYAML(ctx, root, fetcher)
	if err != nil {
		return nil, false, fmt.Errorf("fetching zarf.yaml: %w", err)
	}
	components, err := filter.Apply(pkg)
	if err != nil {
		return nil, false, fmt.Errorf("applying component filter: %w", err)
	}
	layers, err := zoci.AssembleLayers(ctx, root, fetcher, components)
	if err != nil {
		return nil, false, fmt.Errorf("assembling package layers: %w", err)
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
