// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package test contains e2e tests for UDS
package test

import (
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

// ValidateMultiArchIndex validates the given index is a multi-arch index with two manifests
func ValidateMultiArchIndex(t *testing.T, index ocispec.Index) {
	require.Equal(t, 2, len(index.Manifests))
	require.Equal(t, ocispec.MediaTypeImageIndex, index.MediaType)

	var checkedAMD, checkedARM bool
	for _, manifest := range index.Manifests {
		require.Equal(t, ocispec.MediaTypeImageManifest, manifest.MediaType)
		require.Equal(t, "multi", manifest.Platform.OS)
		if manifest.Platform.Architecture == "amd64" {
			require.Equal(t, "amd64", manifest.Platform.Architecture)
			checkedAMD = true
		} else {
			require.Equal(t, "arm64", manifest.Platform.Architecture)
			checkedARM = true
		}
	}
	require.True(t, checkedAMD)
	require.True(t, checkedARM)
}
