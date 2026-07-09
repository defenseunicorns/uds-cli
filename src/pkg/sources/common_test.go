// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package sources contains Zarf packager sources
package sources

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
)

// TestSetAsYOLO ensures YOLO mode strips images, image archives, and repos from every component.
// imageArchives must be stripped too (CLI-235); otherwise a dev/YOLO deploy of a package with an
// imageArchive would try to load images with no Zarf registry present.
func TestSetAsYOLO(t *testing.T) {
	pkg := &v1alpha1.ZarfPackage{
		Components: []v1alpha1.ZarfComponent{
			{
				Name:   "with-images",
				Images: []string{"ghcr.io/defenseunicorns/some-image:1.0.0"},
				Repos:  []string{"https://github.com/defenseunicorns/uds-cli.git"},
			},
			{
				Name: "with-archive",
				ImageArchives: []v1alpha1.ImageArchive{
					{Path: "image.tar", Images: []string{"ghcr.io/uds-packages/tinkerbell/hookos-artifacts:0.1.0"}},
				},
			},
		},
	}

	setAsYOLO(pkg)

	require.True(t, pkg.Metadata.YOLO)
	for _, component := range pkg.Components {
		require.Empty(t, component.Images, "images should be stripped for component %q", component.Name)
		require.Empty(t, component.ImageArchives, "image archives should be stripped for component %q", component.Name)
		require.Empty(t, component.Repos, "repos should be stripped for component %q", component.Name)
	}
}
