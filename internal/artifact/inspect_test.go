// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindPackageManifestRejectsInvalidEntries(t *testing.T) {
	tests := []struct {
		name        string
		manifests   []ociManifest
		wantErrFrag string
	}{
		{
			name: "nested index",
			manifests: []ociManifest{{
				MediaType: ocispec.MediaTypeImageIndex,
				Annotations: map[string]string{
					"org.opencontainers.image.ref.name": "pkg",
				},
			}},
			wantErrFrag: "unsupported media type",
		},
		{
			name: "duplicate manifests",
			manifests: []ociManifest{
				{
					MediaType:   ocispec.MediaTypeImageManifest,
					Annotations: map[string]string{"org.opencontainers.image.ref.name": "pkg"},
				},
				{
					MediaType:   ocispec.MediaTypeImageManifest,
					Annotations: map[string]string{"org.opencontainers.image.ref.name": "pkg"},
				},
			},
			wantErrFrag: "multiple matching",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := findPackageManifest(ociIndex{Manifests: tt.manifests}, spec.Package{Name: "pkg", Source: "pkg"})
			require.ErrorContains(t, err, tt.wantErrFrag)
		})
	}
}

func TestFindPackageManifestMatchesOCIReference(t *testing.T) {
	manifest := ociManifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    "sha256:package",
		Annotations: map[string]string{
			"org.opencontainers.image.ref.name": "example.com/pkg:v1",
		},
	}

	entry, err := findPackageManifest(
		ociIndex{Manifests: []ociManifest{manifest}},
		spec.Package{Name: "pkg", Source: "oci://example.com/pkg:v1"},
	)
	require.NoError(t, err)
	assert.Equal(t, manifest.Digest, entry.Digest)
}

func TestPackageVerificationStatus(t *testing.T) {
	verify := true
	falseValue := false
	tests := []struct {
		name         string
		verification *spec.PackageSignatureVerification
		manifest     *ociManifest
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
			manifest: &ociManifest{Annotations: map[string]string{
				oci.AnnotationPackageVerification: oci.AnnotationPackageVerificationVerified,
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
