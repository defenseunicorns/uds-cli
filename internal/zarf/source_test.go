// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"testing"

	packageoci "github.com/defenseunicorns/pkg/oci"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
)

func TestSelectZarfLayersFiltersBeforeCopy(t *testing.T) {
	store := memory.New()
	push := func(title string, data []byte) ocispec.Descriptor {
		t.Helper()
		desc := content.NewDescriptorFromBytes("application/vnd.zarf.layer.v1.blob", data)
		desc.Annotations = map[string]string{ocispec.AnnotationTitle: title}
		require.NoError(t, udsoci.PushDescriptorBytes(t.Context(), store, desc, data))
		return desc
	}
	zarfYAML := []byte(`kind: ZarfPackageConfig
metadata:
  name: selection
  version: 1.0.0
components:
  - name: required
    required: true
  - name: optional
`)
	root := &packageoci.Manifest{Manifest: ocispec.Manifest{Layers: []ocispec.Descriptor{
		push("zarf.yaml", zarfYAML),
		push("checksums.txt", nil),
		push("components/required.tar", []byte("required")),
		push("components/optional.tar", []byte("optional")),
	}}}

	layers, partial, err := selectZarfLayers(t.Context(), root, store, BuildComponentFilter(nil))
	require.NoError(t, err)
	assert.True(t, partial)
	titles := make([]string, 0, len(layers))
	for _, layer := range layers {
		titles = append(titles, layer.Annotations[ocispec.AnnotationTitle])
	}
	assert.Contains(t, titles, "components/required.tar")
	assert.NotContains(t, titles, "components/optional.tar")

	layers, partial, err = selectZarfLayers(t.Context(), root, store, BuildComponentFilter([]string{"optional"}))
	require.NoError(t, err)
	assert.False(t, partial)
	titles = titles[:0]
	for _, layer := range layers {
		titles = append(titles, layer.Annotations[ocispec.AnnotationTitle])
	}
	assert.Contains(t, titles, "components/required.tar")
	assert.Contains(t, titles, "components/optional.tar")
}

func TestNewPackageSource_Remote(t *testing.T) {
	opts := ConfigOptions{
		Architecture:  "amd64",
		PlainHTTP:     true,
		SkipTLSVerify: true,
		Concurrency:   4,
	}

	tests := []struct {
		name    string
		source  string
		wantRef string
	}{
		{
			name:    "with oci:// scheme",
			source:  "oci://ghcr.io/org/repo:v1.0.0",
			wantRef: "ghcr.io/org/repo:v1.0.0",
		},
		{
			name:    "without scheme",
			source:  "ghcr.io/org/repo:v1.0.0",
			wantRef: "ghcr.io/org/repo:v1.0.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := NewPackageSource(tt.source, opts, "/bundle/dir", iostreams.IOStreams{})
			remote, ok := src.(*remoteSource)
			require.True(t, ok, "expected *remoteSource")
			assert.Equal(t, tt.wantRef, remote.ref)
			assert.Equal(t, "amd64", remote.arch)
			assert.Equal(t, opts, remote.opts)
		})
	}
}

func TestNewPackageSource_Local(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "relative path", source: "./my-package"},
		{name: "tar.zst file", source: "my-package.tar.zst"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ConfigOptions{Architecture: "arm64", TmpDir: "/custom/tmp"}
			src := NewPackageSource(tt.source, opts, "/bundle/dir", iostreams.IOStreams{})
			local, ok := src.(*localSource)
			require.True(t, ok, "expected *localSource")
			assert.Equal(t, tt.source, local.path)
			assert.Equal(t, "arm64", local.arch)
			assert.Equal(t, "/bundle/dir", local.bundleDir)
		})
	}
}
