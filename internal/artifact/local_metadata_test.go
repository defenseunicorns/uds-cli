// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalArchiveMetadataSourceReadsSelectedMetadata(t *testing.T) {
	artifactPath := buildBundleArtifact(t, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = "test-bundle"
  version = "0.1.0"
}
package "selected" {
  source = "selected"
}
package "unselected" {
  source = "unselected"
}
`, nil, []spec.Package{
		{Name: "selected", Source: "selected"},
		{Name: "unselected", Source: "unselected"},
	})

	source, err := OpenLocalArchiveMetadataSource(t.Context(), artifactPath)
	require.NoError(t, err)
	metadata, err := InspectBundleIndex(
		t.Context(),
		iostreams.IOStreams{},
		source.Index,
		source.ArtifactDigest,
		source.Fetcher,
		"selected",
	)
	require.NoError(t, err)
	assert.Equal(t, "test-bundle", metadata.Bundle.Metadata.Name)
	assert.Equal(t, map[string]string{"selected": "selected"}, metadata.PackageZarfNames)
	assert.NotEmpty(t, source.Index)
	assert.False(t, source.SignatureFound)
}
