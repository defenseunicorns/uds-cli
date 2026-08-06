// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
)

// TestGatherComponentImages ensures that `uds inspect --list-images` surfaces images referenced
// via imageArchives, not just those in component.Images. Regression test for CLI-235.
func TestGatherComponentImages(t *testing.T) {
	const (
		archiveImg = "ghcr.io/uds-packages/tinkerbell/hookos-artifacts:0.1.0"
		regularImg = "ghcr.io/defenseunicorns/some-image:1.0.0"
	)

	tests := []struct {
		name       string
		components []v1alpha1.ZarfComponent
		want       []string
	}{
		{
			name:       "no components",
			components: nil,
			want:       nil,
		},
		{
			name: "only imageArchives",
			components: []v1alpha1.ZarfComponent{
				{
					Name: "hookos-artifacts",
					ImageArchives: []v1alpha1.ImageArchive{
						{Path: "image.tar", Images: []string{archiveImg}},
					},
				},
			},
			want: []string{archiveImg},
		},
		{
			name: "images and imageArchives across components",
			components: []v1alpha1.ZarfComponent{
				{Name: "regular", Images: []string{regularImg}},
				{
					Name: "hookos-artifacts",
					ImageArchives: []v1alpha1.ImageArchive{
						{Path: "image.tar", Images: []string{archiveImg}},
					},
				},
			},
			want: []string{regularImg, archiveImg},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ElementsMatch(t, tt.want, gatherComponentImages(tt.components))
		})
	}
}
