// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"encoding/json"
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
	"gopkg.in/yaml.v3"
)

// zarfComponentNameFromTitle extracts the Zarf component name from a layer title.
// Zarf stores component archives as "components/<name>.tar"; returns ("", false)
// for any other title.
func zarfComponentNameFromTitle(title string) (string, bool) {
	name, ok := strings.CutPrefix(title, "components/")
	if !ok || !strings.HasSuffix(name, ".tar") || name == ".tar" {
		return "", false
	}
	return strings.TrimSuffix(name, ".tar"), true
}

// filterOCIManifestsByArch returns only the manifests that match the given
// target architecture. Manifests with no platform info are always included.
// Returns manifests unchanged when arch is empty.
func filterOCIManifestsByArch(manifests []ociManifest, arch string) []ociManifest {
	if arch == "" {
		return manifests
	}
	var filtered []ociManifest
	for _, m := range manifests {
		if m.Platform == nil || m.Platform.Architecture == "" || m.Platform.Architecture == arch {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// isZarfOCIPackage returns true if the root manifest contains a zarf.yaml layer,
// indicating it's a Zarf package that supports component-level filtering.
func isZarfOCIPackage(root *oci.Manifest) bool {
	desc := root.Locate(layout.ZarfYAML)
	return !oci.IsEmptyDescriptor(desc)
}

// filterIngestedManifest rewrites an already-ingested manifest blob to exclude
// layers belonging to Zarf components that the filter excludes.
// Used by localSource.IngestFiltered to apply filtering after walking the
// local Zarf package directory.
//
// Non-Zarf manifests (no zarf.yaml layer) are returned unchanged.
func filterIngestedManifest(ctx context.Context, streams iostreams.IOStreams, blobDir string, m ociManifest, filter filters.ComponentFilterStrategy) (ociManifest, error) {
	md, err := udsoci.ParseDigest(m.Digest)
	if err != nil {
		return ociManifest{}, err
	}

	manifestBytes, err := os.ReadFile(filepath.Join(blobDir, md.Encoded()))
	if err != nil {
		return ociManifest{}, fmt.Errorf("reading manifest blob: %w", err)
	}

	var im ociImageManifest
	if err := json.Unmarshal(manifestBytes, &im); err != nil {
		return ociManifest{}, fmt.Errorf("parsing image manifest: %w", err)
	}

	// Find and parse zarf.yaml from the layers
	var zarfYAMLData []byte
	for _, l := range im.Layers {
		if l.Annotations[zarfLayerTitleAnnotation] != "zarf.yaml" {
			continue
		}
		ld, err := udsoci.ParseDigest(l.Digest)
		if err != nil {
			return ociManifest{}, err
		}
		zarfYAMLData, err = os.ReadFile(filepath.Join(blobDir, ld.Encoded()))
		if err != nil {
			return ociManifest{}, fmt.Errorf("reading zarf.yaml blob: %w", err)
		}
		break
	}
	if zarfYAMLData == nil {
		// Not a Zarf package; pass through unchanged.
		return m, nil
	}

	var pkg v1alpha1.ZarfPackage
	if err := yaml.Unmarshal(zarfYAMLData, &pkg); err != nil {
		return ociManifest{}, fmt.Errorf("parsing zarf.yaml: %w", err)
	}

	// Apply the filter to determine which components to keep
	filteredComponents, err := filter.Apply(pkg)
	if err != nil {
		return ociManifest{}, fmt.Errorf("applying filter: %w", err)
	}
	keepSet := make(map[string]bool, len(filteredComponents))
	for _, c := range filteredComponents {
		keepSet[c.Name] = true
	}

	// Build the set of image blob digests to exclude.
	// Images are only excluded if they are referenced exclusively by filtered-out components.
	excludeImageBlobs, err := imageBlobsToExclude(ctx, streams, blobDir, im.Layers, pkg, keepSet)
	if err != nil {
		return ociManifest{}, fmt.Errorf("resolving image blobs to exclude: %w", err)
	}

	// Filter layers: drop component tarballs not in keepSet and image blobs
	// referenced only by excluded components.
	filtered := make([]ociDescriptor, 0, len(im.Layers))
	for _, l := range im.Layers {
		title := l.Annotations[zarfLayerTitleAnnotation]

		// Exclude component tarballs for filtered-out components
		compName, isComp := zarfComponentNameFromTitle(title)
		if isComp && !keepSet[compName] {
			streams.Debug("excluding component from local package", "component", compName)
			continue
		}

		// Exclude image blobs only used by filtered-out components
		if excludeImageBlobs[l.Digest] {
			streams.Debug("excluding image blob from local package", "title", title)
			continue
		}

		filtered = append(filtered, l)
	}

	if len(filtered) == len(im.Layers) {
		return m, nil
	}

	im.Layers = filtered
	newManifestBytes, err := json.Marshal(im)
	if err != nil {
		return ociManifest{}, fmt.Errorf("marshalling filtered manifest: %w", err)
	}

	newDigest, err := udsoci.WriteAndDigestBlob(blobDir, newManifestBytes)
	if err != nil {
		return ociManifest{}, fmt.Errorf("writing filtered manifest blob: %w", err)
	}

	return ociManifest{
		MediaType:   m.MediaType,
		Digest:      newDigest.String(),
		Size:        int64(len(newManifestBytes)),
		Platform:    m.Platform,
		Annotations: m.Annotations,
	}, nil
}

// imageBlobsToExclude determines which image blob layers can be dropped from a
// local Zarf package manifest when some components are filtered out.
//
// It works by:
//  1. Collecting image refs from kept vs all components
//  2. If no images are being excluded, returning an empty set
//  3. Parsing the images/index.json to map image refs → manifest digests
//  4. Walking each excluded image manifest to collect its blob digests
//  5. Removing any digest that is also referenced by a kept image (shared layers)
//
// Returns a set of layer digest strings that are safe to exclude.
func imageBlobsToExclude(_ context.Context, streams iostreams.IOStreams, blobDir string, layers []ociDescriptor, pkg v1alpha1.ZarfPackage, keepComponents map[string]bool) (map[string]bool, error) {
	// Collect images from kept components and all components
	keptImages := make(map[string]bool)
	allImages := make(map[string]bool)
	for _, c := range pkg.Components {
		for _, img := range c.Images {
			allImages[img] = true
			if keepComponents[c.Name] {
				keptImages[img] = true
			}
		}
	}

	// Determine which images are exclusively used by excluded components
	excludedImages := make(map[string]bool)
	for img := range allImages {
		if !keptImages[img] {
			excludedImages[img] = true
		}
	}
	if len(excludedImages) == 0 {
		return nil, nil
	}

	// Find and read the images/index.json blob
	var indexData []byte
	for _, l := range layers {
		if l.Annotations[zarfLayerTitleAnnotation] == layout.IndexPath {
			ld, err := udsoci.ParseDigest(l.Digest)
			if err != nil {
				return nil, err
			}
			indexData, err = os.ReadFile(filepath.Join(blobDir, ld.Encoded()))
			if err != nil {
				return nil, fmt.Errorf("reading images index blob: %w", err)
			}
			break
		}
	}
	if indexData == nil {
		streams.Debug("images/index.json layer not found in manifest; skipping image blob filtering")
		return nil, nil
	}

	var index ocispec.Index
	if err := json.Unmarshal(indexData, &index); err != nil {
		return nil, fmt.Errorf("parsing images index: %w", err)
	}

	// Map image manifests to kept vs excluded sets
	keptBlobDigests := make(map[string]bool)
	excludedBlobDigests := make(map[string]bool)

	for _, desc := range index.Manifests {
		imageRef := desc.Annotations[ocispec.AnnotationBaseImageName]
		isExcluded := excludedImages[imageRef]
		isKept := keptImages[imageRef]

		// If the image isn't in either set, keep its blobs by skipping
		if !isExcluded && !isKept {
			streams.Debug("image in OCI index not referenced by any component; keeping blobs",
				"imageRef", imageRef, "digest", desc.Digest)
			continue
		}

		manifestPath := filepath.Join(blobDir, desc.Digest.Encoded())
		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("reading image manifest blob %s: %w", desc.Digest, err)
		}

		var im ocispec.Manifest
		if err := json.Unmarshal(manifestData, &im); err != nil {
			return nil, fmt.Errorf("parsing image manifest %s: %w", desc.Digest, err)
		}

		// Collect all blob digests for this image: manifest, config, layers
		blobDigests := []string{desc.Digest.Encoded(), im.Config.Digest.Encoded()}
		for _, l := range im.Layers {
			blobDigests = append(blobDigests, l.Digest.Encoded())
		}

		target := excludedBlobDigests
		if isKept {
			target = keptBlobDigests
		}
		for _, d := range blobDigests {
			target[d] = true
		}
	}

	// Only exclude blobs not also referenced by kept images (shared layers)
	result := make(map[string]bool)
	for d := range excludedBlobDigests {
		if !keptBlobDigests[d] {
			// Store as full digest string to match layer Digest field format
			result["sha256:"+d] = true
		}
	}

	// Also exclude the images/index.json and images/oci-layout layers if ALL
	// images are excluded (no kept images reference any images at all)
	if len(keptImages) == 0 {
		for _, l := range layers {
			title := l.Annotations[zarfLayerTitleAnnotation]
			if title == layout.IndexPath || title == layout.OCILayoutPath {
				result[l.Digest] = true
			}
		}
	}

	return result, nil
}
