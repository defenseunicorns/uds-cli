// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gopkg.in/yaml.v3"
)

type blobFetcher func(context.Context, ocispec.Descriptor) ([]byte, error)
type packageMetadata struct {
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Build struct {
		Signed *bool `yaml:"signed"`
	} `yaml:"build"`
}

func readPackageZarfNames(ctx context.Context, manifests map[string]ocispec.Descriptor, fetch blobFetcher) (map[string]string, error) {
	packageNames := make([]string, 0, len(manifests))
	for packageName := range manifests {
		packageNames = append(packageNames, packageName)
	}
	sort.Strings(packageNames)
	zarfNames := make(map[string]string, len(manifests))
	for _, packageName := range packageNames {
		metadata, err := readPackageMetadata(ctx, packageName, manifests[packageName], fetch)
		if err != nil {
			return nil, err
		}
		if metadata.Metadata.Name == "" {
			return nil, MissingZarfPackageNameError{Package: packageName}
		}
		zarfNames[packageName] = metadata.Metadata.Name
	}
	return zarfNames, nil
}
func readPackageMetadata(ctx context.Context, packageName string, entry ocispec.Descriptor, fetch blobFetcher) (*packageMetadata, error) {
	manifestBytes, err := fetch(ctx, entry)
	if err != nil {
		return nil, fmt.Errorf("%w %s for package %q: %w", ErrFetchingPackageManifest, entry.Digest, packageName, err)
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("%w %s for package %q: %w", ErrParsingPackageManifest, entry.Digest, packageName, err)
	}
	if manifest.SchemaVersion != 2 {
		return nil, UnsupportedSchemaVersionError{Artifact: "package manifest", Version: manifest.SchemaVersion}
	}
	if manifest.MediaType != "" && !udsoci.IsImageManifestMediaType(manifest.MediaType) {
		return nil, UnsupportedMediaTypeError{Artifact: "package manifest", MediaType: manifest.MediaType}
	}
	metadata := &packageMetadata{}
	zarfLayer, ok := findLayerByTitleOptional(manifest, "zarf.yaml")
	if !ok {
		return metadata, nil
	}
	zarfBytes, err := fetch(ctx, zarfLayer)
	if err != nil {
		return nil, fmt.Errorf("%w %s for package %q: %w", ErrFetchingZarfYAML, zarfLayer.Digest, packageName, err)
	}
	if err := yaml.Unmarshal(zarfBytes, metadata); err != nil {
		return nil, fmt.Errorf("%w %s for package %q: %w", ErrParsingZarfYAML, zarfLayer.Digest, packageName, err)
	}
	return metadata, nil
}
