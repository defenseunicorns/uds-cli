// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundler defines behavior for bundling packages
package bundler

import (
	"errors"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
)

func platformForBundle(bundle *types.UDSBundle) (ocispec.Platform, error) {
	if bundle.Metadata.Architecture == "" {
		return ocispec.Platform{}, errors.New("architecture is required for bundling")
	}
	return ocispec.Platform{Architecture: bundle.Build.Architecture, OS: oci.MultiOS}, nil
}

func manifestConfigAnnotationsFromMetadata(metadata *types.UDSMetadata) map[string]string {
	return map[string]string{
		ocispec.AnnotationTitle:       metadata.Name,
		ocispec.AnnotationDescription: metadata.Description,
	}
}

// manifestAnnotationsFromMetadata maps UDS metadata to standard OCI annotations.
func manifestAnnotationsFromMetadata(metadata *types.UDSMetadata) map[string]string {
	annotations := map[string]string{
		ocispec.AnnotationDescription: metadata.Description,
	}

	if url := metadata.URL; url != "" {
		annotations[ocispec.AnnotationURL] = url
	}
	if authors := metadata.Authors; authors != "" {
		annotations[ocispec.AnnotationAuthors] = authors
	}
	if documentation := metadata.Documentation; documentation != "" {
		annotations[ocispec.AnnotationDocumentation] = documentation
	}
	if source := metadata.Source; source != "" {
		annotations[ocispec.AnnotationSource] = source
	}
	if vendor := metadata.Vendor; vendor != "" {
		annotations[ocispec.AnnotationVendor] = vendor
	}

	return annotations
}

// referenceFromMetadata builds the canonical OCI publishing reference for a UDS bundle.
func referenceFromMetadata(registryLocation string, metadata *types.UDSMetadata) (string, error) {
	pkg := v1alpha1.ZarfPackage{
		Metadata: v1alpha1.ZarfMetadata{
			Name:    metadata.Name,
			Version: metadata.Version,
		},
	}
	ref, err := zoci.ReferenceFromMetadata(registryLocation, pkg)
	if err != nil {
		return "", err
	}
	return ref.String(), nil
}
