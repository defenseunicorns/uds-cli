// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundler

import (
	"testing"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

func TestReferenceFromMetadataBuildsCanonicalBundleReference(t *testing.T) {
	ref, err := referenceFromMetadata("oci://ghcr.io/example/bundles", &types.UDSMetadata{
		Name:    "demo",
		Version: "1.2.3",
	})
	require.NoError(t, err)
	require.Equal(t, "ghcr.io/example/bundles/demo:1.2.3", ref)
}

func TestReferenceFromMetadataRequiresVersion(t *testing.T) {
	_, err := referenceFromMetadata("oci://ghcr.io/example/bundles", &types.UDSMetadata{Name: "demo"})
	require.EqualError(t, err, "version is required for publishing")
}

func TestPlatformForBundleUsesResolvedBuildArchitecture(t *testing.T) {
	platform, err := platformForBundle(&types.UDSBundle{
		Metadata: types.UDSMetadata{Architecture: "amd64"},
		Build:    types.UDSBuildData{Architecture: "arm64"},
	})
	require.NoError(t, err)
	require.Equal(t, ocispec.Platform{Architecture: "arm64", OS: oci.MultiOS}, platform)
}

func TestPlatformForBundleRequiresMetadataArchitecture(t *testing.T) {
	_, err := platformForBundle(&types.UDSBundle{Build: types.UDSBuildData{Architecture: "amd64"}})
	require.EqualError(t, err, "architecture is required for bundling")
}

func TestManifestConfigAnnotationsFromMetadata(t *testing.T) {
	annotations := manifestConfigAnnotationsFromMetadata(&types.UDSMetadata{
		Name:        "demo",
		Description: "Demo bundle",
	})
	require.Equal(t, map[string]string{
		ocispec.AnnotationTitle:       "demo",
		ocispec.AnnotationDescription: "Demo bundle",
	}, annotations)
}
