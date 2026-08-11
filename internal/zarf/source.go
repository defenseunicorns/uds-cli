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
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	"gopkg.in/yaml.v3"
)

// zarfLayerTitleAnnotation is the OCI annotation key for the file path within a Zarf package.
// Zarf stores each package file as a raw-bytes OCI layer, with this annotation
// set to the file's relative path (e.g. "zarf.yaml", "components/my-pkg.tar").
const zarfLayerTitleAnnotation = "org.opencontainers.image.title"

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

// readZarfMetadata reads and parses metadata from a zarf.yaml file.
// Returns an empty zarfMetadata if the file doesn't exist or cannot be parsed.
func readZarfMetadata(zarfYamlPath string) zarfMetadata {
	var meta zarfMetadata
	if data, err := os.ReadFile(zarfYamlPath); err == nil {
		_ = yaml.Unmarshal(data, &meta)
	}
	return meta
}

// isZarfPackage checks if the directory is a Zarf package by looking for zarf.yaml
func isZarfPackage(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "zarf.yaml"))
	return err == nil
}

// buildFilteredLayerList assembles the minimal set of OCI layer descriptors
// for a Zarf package pull, including only layers for the filtered components
// and their referenced container images.
//
// This replaces the previous approach of pulling ALL layers (via zoci.AllLayers)
// and filtering after download.
//
// The returned descriptors include:
//   - Metadata layers (zarf.yaml, checksums.txt, zarf.yaml.sig) — always pulled
//   - Component tarballs for the filtered components
//   - Container image blobs referenced by those components
//   - SBOMs tarball (if present)
//   - Documentation tarball (if present and pkg has documentation)
func buildFilteredLayerList(ctx context.Context, remote *zoci.Remote, root *oci.Manifest, pkg v1alpha1.ZarfPackage) ([]ocispec.Descriptor, error) {
	var all []ocispec.Descriptor

	// 1. Metadata layers (always pulled)
	for _, path := range zoci.PackageAlwaysPull {
		desc := root.Locate(path)
		if !oci.IsEmptyDescriptor(desc) {
			all = append(all, desc)
		}
	}

	// 2. Component layers for filtered components
	compLayers, imageRefs, err := remote.LayersFromComponents(ctx, pkg, pkg.Components)
	if err != nil {
		return nil, fmt.Errorf("resolving component layers: %w", err)
	}
	all = append(all, compLayers...)

	// 3. Container image layers referenced by the filtered components
	if len(imageRefs) > 0 {
		imgLayers, err := remote.LayersFromImages(ctx, imageRefs)
		if err != nil {
			return nil, fmt.Errorf("resolving image layers: %w", err)
		}
		all = append(all, imgLayers...)
	}

	// 4. SBOMs
	sbomDesc := root.Locate(layout.SBOMTar)
	if !oci.IsEmptyDescriptor(sbomDesc) {
		all = append(all, sbomDesc)
	}

	// 5. Documentation
	if len(pkg.Documentation) > 0 {
		docDesc := root.Locate(layout.DocumentationTar)
		if !oci.IsEmptyDescriptor(docDesc) {
			all = append(all, docDesc)
		}
	}

	return oci.RemoveDuplicateDescriptors(all), nil
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
