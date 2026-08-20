// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"testing"

	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindPackageManifestRejectsInvalidEntries(t *testing.T) {
	tests := []struct {
		name        string
		manifests   []ocispec.Descriptor
		wantErrFrag string
	}{
		{
			name: "nested index",
			manifests: []ocispec.Descriptor{{
				MediaType: ocispec.MediaTypeImageIndex,
				Annotations: map[string]string{
					udsoci.AnnotationPackageName: "pkg",
					ocispec.AnnotationRefName:    "pkg",
				},
			}},
			wantErrFrag: "unsupported media type",
		},
		{
			name: "legacy ref name only",
			manifests: []ocispec.Descriptor{{
				MediaType:   ocispec.MediaTypeImageManifest,
				Annotations: map[string]string{ocispec.AnnotationRefName: "pkg"},
			}},
			wantErrFrag: "no uds.dev/package.name",
		},
		{
			name: "duplicate manifests",
			manifests: []ocispec.Descriptor{
				{
					MediaType:   ocispec.MediaTypeImageManifest,
					Annotations: map[string]string{udsoci.AnnotationPackageName: "pkg", ocispec.AnnotationRefName: "pkg"},
				},
				{
					MediaType:   ocispec.MediaTypeImageManifest,
					Annotations: map[string]string{udsoci.AnnotationPackageName: "pkg", ocispec.AnnotationRefName: "pkg"},
				},
			},
			wantErrFrag: "multiple matching",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := findPackageManifest(ocispec.Index{Manifests: tt.manifests}, spec.Package{Name: "pkg", Source: "pkg"})
			require.ErrorContains(t, err, tt.wantErrFrag)
			switch tt.name {
			case "nested index":
				var mediaTypeErr UnsupportedPackageEntryMediaTypeError
				require.ErrorAs(t, err, &mediaTypeErr)
				assert.Equal(t, "pkg", mediaTypeErr.Package)
				assert.Equal(t, ocispec.MediaTypeImageIndex, mediaTypeErr.MediaType)
			case "legacy ref name only":
				var annotationErr MissingPackageManifestAnnotationError
				require.ErrorAs(t, err, &annotationErr)
				assert.Equal(t, udsoci.AnnotationPackageName, annotationErr.Annotation)
			case "duplicate manifests":
				var multipleErr MultiplePackageManifestEntriesError
				require.ErrorAs(t, err, &multipleErr)
				assert.Equal(t, "pkg", multipleErr.Package)
			}
		})
	}
}

func TestFindPackageManifestMatchesPackageName(t *testing.T) {
	manifest := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Annotations: map[string]string{
			udsoci.AnnotationPackageName:   "pkg",
			udsoci.AnnotationPackageSource: "oci://example.com/pkg:v1",
			ocispec.AnnotationRefName:      "pkg",
		},
	}

	entry, err := findPackageManifest(
		ocispec.Index{Manifests: []ocispec.Descriptor{manifest}},
		spec.Package{Name: "pkg", Source: "oci://example.com/pkg:v1"},
	)
	require.NoError(t, err)
	assert.Equal(t, manifest.MediaType, entry.MediaType)
}

func TestPackageVerificationStatus(t *testing.T) {
	verify := true
	falseValue := false
	tests := []struct {
		name         string
		verification *spec.PackageSignatureVerification
		manifest     *ocispec.Descriptor
		want         PackageVerificationStatus
	}{
		{name: "missing", want: PackageVerificationStatusUnknown},
		{
			name:         "disabled",
			verification: &spec.PackageSignatureVerification{Verify: &falseValue},
			want:         PackageVerificationStatusSkipped,
		},
		{
			name:         "enabled",
			verification: &spec.PackageSignatureVerification{Verify: &verify},
			want:         PackageVerificationStatusUnknown,
		},
		{
			name: "persisted verified",
			manifest: &ocispec.Descriptor{Annotations: map[string]string{
				udsoci.AnnotationPackageVerification: udsoci.AnnotationPackageVerificationVerified,
			}},
			want: PackageVerificationStatusVerified,
		},
		{
			name:         "default enabled",
			verification: &spec.PackageSignatureVerification{},
			want:         PackageVerificationStatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, packageVerificationStatus(tt.verification, tt.manifest))
		})
	}
}
