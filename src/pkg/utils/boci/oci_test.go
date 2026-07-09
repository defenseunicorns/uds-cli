// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package boci

import (
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
)

// manifestFor builds an image manifest descriptor annotated with the given base image name.
func manifestFor(imgName string) ocispec.Descriptor {
	return ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromString(imgName),
		Annotations: map[string]string{
			ocispec.AnnotationBaseImageName: imgName,
		},
	}
}

// TestFilterImageIndex ensures that images referenced only via imageArchives are retained when
// filtering the image index. Regression test for CLI-235: deploying an OCI bundle whose Zarf
// package declares an imageArchive previously failed with "checksum missing in layout" because
// FilterImageIndex only inspected component.Images and dropped the archive's manifest/blobs.
func TestFilterImageIndex(t *testing.T) {
	const (
		archiveImg = "ghcr.io/uds-packages/tinkerbell/hookos-artifacts:0.1.0"
		regularImg = "ghcr.io/defenseunicorns/some-image:1.0.0"
		unusedImg  = "ghcr.io/defenseunicorns/not-referenced:2.0.0"
	)

	index := ocispec.Index{
		Manifests: []ocispec.Descriptor{
			manifestFor(archiveImg),
			manifestFor(regularImg),
			manifestFor(unusedImg),
		},
	}

	tests := []struct {
		name       string
		components []v1alpha1.ZarfComponent
		wantImages []string
	}{
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
			wantImages: []string{archiveImg},
		},
		{
			name: "only images",
			components: []v1alpha1.ZarfComponent{
				{Name: "regular", Images: []string{regularImg}},
			},
			wantImages: []string{regularImg},
		},
		{
			name: "both images and imageArchives",
			components: []v1alpha1.ZarfComponent{
				{
					Name:   "mixed",
					Images: []string{regularImg},
					ImageArchives: []v1alpha1.ImageArchive{
						{Path: "image.tar", Images: []string{archiveImg}},
					},
				},
			},
			wantImages: []string{regularImg, archiveImg},
		},
		{
			name: "no matching images",
			components: []v1alpha1.ZarfComponent{
				{Name: "empty"},
			},
			wantImages: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifests, err := FilterImageIndex(tt.components, index)
			require.NoError(t, err)

			gotDigests := make([]string, 0, len(manifests))
			for _, m := range manifests {
				gotDigests = append(gotDigests, m.Digest.String())
			}

			wantDigests := make([]string, 0, len(tt.wantImages))
			for _, img := range tt.wantImages {
				wantDigests = append(wantDigests, digest.FromString(img).String())
			}

			require.ElementsMatch(t, wantDigests, gotDigests)
		})
	}
}
