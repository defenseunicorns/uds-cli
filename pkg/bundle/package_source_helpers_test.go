// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"gopkg.in/yaml.v3"
)

// fakeDigest returns a valid digest for use in test descriptors.
// Locate() calls Digest.Encoded() on every layer, which panics on empty digests.
func fakeDigest(s string) godigest.Digest {
	return godigest.FromString(s)
}

func TestIsZarfOCIPackage(t *testing.T) {
	tests := []struct {
		name   string
		layers []ocispec.Descriptor
		want   bool
	}{
		{
			name:   "empty manifest",
			layers: nil,
			want:   false,
		},
		{
			name: "manifest with zarf.yaml",
			layers: []ocispec.Descriptor{
				{
					MediaType: "application/vnd.defenseunicorns.zarf.layer.v1",
					Digest:    fakeDigest("zarf-yaml-content"),
					Annotations: map[string]string{
						ocispec.AnnotationTitle: "zarf.yaml",
					},
				},
			},
			want: true,
		},
		{
			name: "manifest without zarf.yaml",
			layers: []ocispec.Descriptor{
				{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    fakeDigest("some-content"),
					Annotations: map[string]string{
						ocispec.AnnotationTitle: "some-file.tar",
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := &oci.Manifest{
				Manifest: ocispec.Manifest{Layers: tt.layers},
			}
			got := isZarfOCIPackage(root)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestZarfComponentNameFromTitle(t *testing.T) {
	tests := []struct {
		title    string
		wantName string
		wantOK   bool
	}{
		{"components/my-pkg.tar", "my-pkg", true},
		{"components/foo-bar.tar", "foo-bar", true},
		{"zarf.yaml", "", false},
		{"checksums.txt", "", false},
		{"components/", "", false},
		{"components/.tar", "", false},
		{"images/index.json", "", false},
	}
	for _, tc := range tests {
		name, ok := zarfComponentNameFromTitle(tc.title)
		assert.Equal(t, tc.wantOK, ok, "title=%q", tc.title)
		assert.Equal(t, tc.wantName, name, "title=%q", tc.title)
	}
}

func TestFilterOCIManifestsByArch(t *testing.T) {
	amd64 := &ocispec.Platform{OS: "linux", Architecture: "amd64"}
	arm64 := &ocispec.Platform{OS: "linux", Architecture: "arm64"}

	manifests := []ociManifest{
		{Platform: amd64},
		{Platform: arm64},
		{Platform: nil},
		{Platform: &ocispec.Platform{Architecture: ""}},
	}

	t.Run("empty arch returns all", func(t *testing.T) {
		got := filterOCIManifestsByArch(manifests, "")
		require.Equal(t, manifests, got)
	})

	t.Run("amd64 keeps amd64 and nil-platform", func(t *testing.T) {
		got := filterOCIManifestsByArch(manifests, "amd64")
		require.Len(t, got, 3)
		assert.Equal(t, amd64, got[0].Platform)
		assert.Nil(t, got[1].Platform)
	})

	t.Run("arm64 keeps arm64 and nil-platform", func(t *testing.T) {
		got := filterOCIManifestsByArch(manifests, "arm64")
		require.Len(t, got, 3)
		assert.Equal(t, arm64, got[0].Platform)
	})

	t.Run("no matches returns nil slice", func(t *testing.T) {
		onlyAMD := []ociManifest{{Platform: amd64}}
		got := filterOCIManifestsByArch(onlyAMD, "arm64")
		require.Empty(t, got)
	})
}

// writeBlobJSON is a test helper that marshals v to JSON, writes it as a
// content-addressed blob to blobDir, and returns its "sha256:<hex>" digest string.
func writeBlobJSON(t *testing.T, blobDir string, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return writeBlobBytes(t, blobDir, b)
}

func writeBlobBytes(t *testing.T, blobDir string, b []byte) string {
	t.Helper()
	sum := sha256.Sum256(b)
	h := hex.EncodeToString(sum[:])
	require.NoError(t, os.WriteFile(filepath.Join(blobDir, h), b, tmpFilePerm))
	return "sha256:" + h
}

// buildFilterTestManifest creates a blob directory with a Zarf-like OCI manifest
// containing zarf.yaml, component tarballs, and optionally image blobs. Returns
// the ociManifest descriptor and blob directory path.
func buildFilterTestManifest(t *testing.T, pkg v1alpha1.ZarfPackage, imageIndex *ocispec.Index, imageManifests map[string]ocispec.Manifest) (ociManifest, string) {
	t.Helper()
	blobDir := filepath.Join(t.TempDir(), "blobs")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

	// Write zarf.yaml blob
	zarfData, err := yaml.Marshal(pkg)
	require.NoError(t, err)
	zarfDigest := writeBlobBytes(t, blobDir, zarfData)

	// Build layers starting with zarf.yaml
	layers := []ociDescriptor{
		{
			MediaType:   MediaTypeZarfLayer,
			Digest:      zarfDigest,
			Size:        int64(len(zarfData)),
			Annotations: map[string]string{zarfLayerTitleAnnotation: "zarf.yaml"},
		},
	}

	// Add component tarball layers for all components
	for _, c := range pkg.Components {
		compData := []byte("component-data-" + c.Name)
		compDigest := writeBlobBytes(t, blobDir, compData)
		layers = append(layers, ociDescriptor{
			MediaType:   MediaTypeZarfLayer,
			Digest:      compDigest,
			Size:        int64(len(compData)),
			Annotations: map[string]string{zarfLayerTitleAnnotation: "components/" + c.Name + ".tar"},
		})
	}

	// Add image blobs if provided
	if imageIndex != nil {
		// Write each image manifest blob and collect image blob layers
		for digestStr, im := range imageManifests {
			imData, err := json.Marshal(im)
			require.NoError(t, err)
			// Write the manifest blob using its expected digest
			d, err := godigest.Parse(digestStr)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(blobDir, d.Encoded()), imData, tmpFilePerm))

			// Write config blob
			configData := []byte("config-" + digestStr)
			require.NoError(t, os.WriteFile(filepath.Join(blobDir, im.Config.Digest.Encoded()), configData, tmpFilePerm))

			// Write layer blobs and create layer descriptors
			for _, l := range im.Layers {
				layerData := []byte("layer-" + l.Digest.Encoded())
				require.NoError(t, os.WriteFile(filepath.Join(blobDir, l.Digest.Encoded()), layerData, tmpFilePerm))

				layers = append(layers, ociDescriptor{
					MediaType:   MediaTypeZarfLayer,
					Digest:      l.Digest.String(),
					Size:        int64(len(layerData)),
					Annotations: map[string]string{zarfLayerTitleAnnotation: "images/blobs/sha256/" + l.Digest.Encoded()},
				})
			}

			// Add image manifest blob as a layer
			layers = append(layers, ociDescriptor{
				MediaType:   MediaTypeZarfLayer,
				Digest:      digestStr,
				Size:        int64(len(imData)),
				Annotations: map[string]string{zarfLayerTitleAnnotation: "images/blobs/sha256/" + d.Encoded()},
			})

			// Add config blob as a layer
			layers = append(layers, ociDescriptor{
				MediaType:   MediaTypeZarfLayer,
				Digest:      im.Config.Digest.String(),
				Size:        int64(len(configData)),
				Annotations: map[string]string{zarfLayerTitleAnnotation: "images/blobs/sha256/" + im.Config.Digest.Encoded()},
			})
		}

		// Write images/index.json
		indexData, err := json.Marshal(imageIndex)
		require.NoError(t, err)
		indexDigest := writeBlobBytes(t, blobDir, indexData)
		layers = append(layers, ociDescriptor{
			MediaType:   MediaTypeZarfLayer,
			Digest:      indexDigest,
			Size:        int64(len(indexData)),
			Annotations: map[string]string{zarfLayerTitleAnnotation: "images/index.json"},
		})

		// Write images/oci-layout
		ociLayoutData := []byte(`{"imageLayoutVersion":"1.0.0"}`)
		ociLayoutDigest := writeBlobBytes(t, blobDir, ociLayoutData)
		layers = append(layers, ociDescriptor{
			MediaType:   MediaTypeZarfLayer,
			Digest:      ociLayoutDigest,
			Size:        int64(len(ociLayoutData)),
			Annotations: map[string]string{zarfLayerTitleAnnotation: "images/oci-layout"},
		})
	}

	// Write the image manifest blob
	im := ociImageManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config: ociDescriptor{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    writeBlobBytes(t, blobDir, []byte(`{"architecture":"amd64"}`)),
			Size:      25,
		},
		Layers: layers,
	}
	manifestDigest := writeBlobJSON(t, blobDir, im)

	return ociManifest{
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Digest:    manifestDigest,
		Size:      100,
	}, blobDir
}

func TestFilterIngestedManifest_NonZarfPassthrough(t *testing.T) {
	// Manifest without zarf.yaml should pass through unchanged
	blobDir := filepath.Join(t.TempDir(), "blobs")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

	im := ociImageManifest{
		SchemaVersion: 2,
		Config:        ociDescriptor{Digest: writeBlobBytes(t, blobDir, []byte(`{}`)), Size: 2},
		Layers: []ociDescriptor{
			{Digest: writeBlobBytes(t, blobDir, []byte("layer1")), Annotations: map[string]string{zarfLayerTitleAnnotation: "some-file.txt"}},
		},
	}
	manifestDigest := writeBlobJSON(t, blobDir, im)

	m := ociManifest{Digest: manifestDigest, Size: 100}
	result, err := filterIngestedManifest(context.Background(), iostreams.IOStreams{}, blobDir, m, filters.Empty())
	require.NoError(t, err)
	assert.Equal(t, m, result, "non-Zarf manifest should be returned unchanged")
}

func TestFilterIngestedManifest_KeepsAllWhenNoFiltering(t *testing.T) {
	reqTrue := true
	pkg := v1alpha1.ZarfPackage{
		Components: []v1alpha1.ZarfComponent{
			{Name: "core", Required: &reqTrue},
			{Name: "logging"},
		},
	}

	m, blobDir := buildFilterTestManifest(t, pkg, nil, nil)

	// ForDeploy with empty string and non-interactive keeps required + default:true
	// Since "logging" has no Required/Default set, it will be excluded by ForDeploy
	// Use Empty() filter to keep all
	result, err := filterIngestedManifest(context.Background(), iostreams.IOStreams{}, blobDir, m, filters.Empty())
	require.NoError(t, err)
	assert.Equal(t, m, result, "should be unchanged when filter keeps all components")
}

func TestFilterIngestedManifest_ExcludesOptionalComponents(t *testing.T) {
	reqTrue := true
	pkg := v1alpha1.ZarfPackage{
		Components: []v1alpha1.ZarfComponent{
			{Name: "core", Required: &reqTrue},
			{Name: "optional-a"},
			{Name: "optional-b"},
		},
	}

	m, blobDir := buildFilterTestManifest(t, pkg, nil, nil)

	// ForDeploy with "optional-a" keeps required + optional-a, excludes optional-b
	filter := filters.ForDeploy("optional-a", false)
	result, err := filterIngestedManifest(context.Background(), iostreams.IOStreams{}, blobDir, m, filter)
	require.NoError(t, err)

	// Result should have different digest since layers were filtered
	assert.NotEqual(t, m.Digest, result.Digest)

	// Read back the filtered manifest to verify layers
	rd, err := godigest.Parse(result.Digest)
	require.NoError(t, err)
	filteredData, err := os.ReadFile(filepath.Join(blobDir, rd.Encoded()))
	require.NoError(t, err)
	var filteredManifest ociImageManifest
	require.NoError(t, json.Unmarshal(filteredData, &filteredManifest))

	// Check that optional-b component tarball is excluded
	titles := make([]string, 0, len(filteredManifest.Layers))
	for _, l := range filteredManifest.Layers {
		titles = append(titles, l.Annotations[zarfLayerTitleAnnotation])
	}
	assert.Contains(t, titles, "zarf.yaml")
	assert.Contains(t, titles, "components/core.tar")
	assert.Contains(t, titles, "components/optional-a.tar")
	assert.NotContains(t, titles, "components/optional-b.tar")
}

func TestFilterIngestedManifest_ExcludesImageBlobs(t *testing.T) {
	reqTrue := true
	pkg := v1alpha1.ZarfPackage{
		Components: []v1alpha1.ZarfComponent{
			{Name: "core", Required: &reqTrue, Images: []string{"registry.example.com/core-image:v1"}},
			{Name: "optional-a", Images: []string{"registry.example.com/optional-image:v1"}},
		},
	}

	// Create image manifests with distinct layer digests
	coreImageLayerDigest := godigest.FromString("core-image-layer")
	coreImageConfigDigest := godigest.FromString("core-image-config")
	coreImageManifestDigest := godigest.FromString("core-image-manifest")

	optionalImageLayerDigest := godigest.FromString("optional-image-layer")
	optionalImageConfigDigest := godigest.FromString("optional-image-config")
	optionalImageManifestDigest := godigest.FromString("optional-image-manifest")

	imageIndex := &ocispec.Index{
		Manifests: []ocispec.Descriptor{
			{
				Digest:    coreImageManifestDigest,
				MediaType: "application/vnd.oci.image.manifest.v1+json",
				Annotations: map[string]string{
					ocispec.AnnotationBaseImageName: "registry.example.com/core-image:v1",
				},
			},
			{
				Digest:    optionalImageManifestDigest,
				MediaType: "application/vnd.oci.image.manifest.v1+json",
				Annotations: map[string]string{
					ocispec.AnnotationBaseImageName: "registry.example.com/optional-image:v1",
				},
			},
		},
	}

	imageManifests := map[string]ocispec.Manifest{
		coreImageManifestDigest.String(): {
			Config: ocispec.Descriptor{Digest: coreImageConfigDigest},
			Layers: []ocispec.Descriptor{{Digest: coreImageLayerDigest}},
		},
		optionalImageManifestDigest.String(): {
			Config: ocispec.Descriptor{Digest: optionalImageConfigDigest},
			Layers: []ocispec.Descriptor{{Digest: optionalImageLayerDigest}},
		},
	}

	m, blobDir := buildFilterTestManifest(t, pkg, imageIndex, imageManifests)

	// Filter: keep only "core" (required), exclude "optional-a"
	filter := filters.ForDeploy("", false)
	result, err := filterIngestedManifest(context.Background(), iostreams.IOStreams{}, blobDir, m, filter)
	require.NoError(t, err)
	assert.NotEqual(t, m.Digest, result.Digest)

	// Read back filtered manifest
	rd, err := godigest.Parse(result.Digest)
	require.NoError(t, err)
	filteredData, err := os.ReadFile(filepath.Join(blobDir, rd.Encoded()))
	require.NoError(t, err)
	var filteredManifest ociImageManifest
	require.NoError(t, json.Unmarshal(filteredData, &filteredManifest))

	// Collect remaining digests
	remainingDigests := make(map[string]bool)
	for _, l := range filteredManifest.Layers {
		remainingDigests[l.Digest] = true
	}

	// Core image blobs should be present
	assert.True(t, remainingDigests[coreImageLayerDigest.String()], "core image layer should be kept")
	assert.True(t, remainingDigests[coreImageConfigDigest.String()], "core image config should be kept")
	assert.True(t, remainingDigests[coreImageManifestDigest.String()], "core image manifest should be kept")

	// Optional image blobs should be excluded
	assert.False(t, remainingDigests[optionalImageLayerDigest.String()], "optional image layer should be excluded")
	assert.False(t, remainingDigests[optionalImageConfigDigest.String()], "optional image config should be excluded")
	assert.False(t, remainingDigests[optionalImageManifestDigest.String()], "optional image manifest should be excluded")

	// Component tarballs: core kept, optional-a excluded
	titles := make([]string, 0)
	for _, l := range filteredManifest.Layers {
		if title := l.Annotations[zarfLayerTitleAnnotation]; title != "" {
			titles = append(titles, title)
		}
	}
	assert.Contains(t, titles, "components/core.tar")
	assert.NotContains(t, titles, "components/optional-a.tar")
}

func TestFilterIngestedManifest_SharedImageBlobsPreserved(t *testing.T) {
	// When two components share an image, filtering one should keep the shared blobs
	reqTrue := true
	sharedImage := "registry.example.com/shared:v1"
	pkg := v1alpha1.ZarfPackage{
		Components: []v1alpha1.ZarfComponent{
			{Name: "core", Required: &reqTrue, Images: []string{sharedImage}},
			{Name: "optional", Images: []string{sharedImage}},
		},
	}

	sharedLayerDigest := godigest.FromString("shared-layer")
	sharedConfigDigest := godigest.FromString("shared-config")
	sharedManifestDigest := godigest.FromString("shared-manifest")

	imageIndex := &ocispec.Index{
		Manifests: []ocispec.Descriptor{
			{
				Digest:    sharedManifestDigest,
				MediaType: "application/vnd.oci.image.manifest.v1+json",
				Annotations: map[string]string{
					ocispec.AnnotationBaseImageName: sharedImage,
				},
			},
		},
	}

	imageManifests := map[string]ocispec.Manifest{
		sharedManifestDigest.String(): {
			Config: ocispec.Descriptor{Digest: sharedConfigDigest},
			Layers: []ocispec.Descriptor{{Digest: sharedLayerDigest}},
		},
	}

	m, blobDir := buildFilterTestManifest(t, pkg, imageIndex, imageManifests)

	// Exclude "optional" — but the image is shared with "core"
	filter := filters.ForDeploy("", false)
	result, err := filterIngestedManifest(context.Background(), iostreams.IOStreams{}, blobDir, m, filter)
	require.NoError(t, err)

	// Read back filtered manifest
	rd, err := godigest.Parse(result.Digest)
	require.NoError(t, err)
	filteredData, err := os.ReadFile(filepath.Join(blobDir, rd.Encoded()))
	require.NoError(t, err)
	var filteredManifest ociImageManifest
	require.NoError(t, json.Unmarshal(filteredData, &filteredManifest))

	remainingDigests := make(map[string]bool)
	for _, l := range filteredManifest.Layers {
		remainingDigests[l.Digest] = true
	}

	// Shared image blobs should still be present since "core" uses them
	assert.True(t, remainingDigests[sharedLayerDigest.String()], "shared image layer should be kept")
	assert.True(t, remainingDigests[sharedConfigDigest.String()], "shared image config should be kept")
	assert.True(t, remainingDigests[sharedManifestDigest.String()], "shared image manifest should be kept")
}

// --- Direct unit tests for imageBlobsToExclude ---

// setupImageBlobsTestFixture creates a blobDir with image manifests and an images/index.json layer,
// returning the layers list and blobDir path.
func setupImageBlobsTestFixture(t *testing.T, imageIndex ocispec.Index, imageManifests map[string]ocispec.Manifest) ([]ociDescriptor, string) {
	t.Helper()
	blobDir := filepath.Join(t.TempDir(), "blobs")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

	var layers []ociDescriptor

	for digestStr, im := range imageManifests {
		imData, err := json.Marshal(im)
		require.NoError(t, err)
		d, err := godigest.Parse(digestStr)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(blobDir, d.Encoded()), imData, tmpFilePerm))

		// Write config blob
		configData := []byte("config-" + digestStr)
		require.NoError(t, os.WriteFile(filepath.Join(blobDir, im.Config.Digest.Encoded()), configData, tmpFilePerm))

		// Write layer blobs
		for _, l := range im.Layers {
			layerData := []byte("layer-" + l.Digest.Encoded())
			require.NoError(t, os.WriteFile(filepath.Join(blobDir, l.Digest.Encoded()), layerData, tmpFilePerm))

			layers = append(layers, ociDescriptor{
				Digest:      l.Digest.String(),
				Annotations: map[string]string{zarfLayerTitleAnnotation: "images/blobs/sha256/" + l.Digest.Encoded()},
			})
		}

		layers = append(layers, ociDescriptor{
			Digest:      digestStr,
			Annotations: map[string]string{zarfLayerTitleAnnotation: "images/blobs/sha256/" + d.Encoded()},
		})
		layers = append(layers, ociDescriptor{
			Digest:      im.Config.Digest.String(),
			Annotations: map[string]string{zarfLayerTitleAnnotation: "images/blobs/sha256/" + im.Config.Digest.Encoded()},
		})
	}

	// Write images/index.json
	indexData, err := json.Marshal(imageIndex)
	require.NoError(t, err)
	indexDigest := writeBlobBytes(t, blobDir, indexData)
	layers = append(layers, ociDescriptor{
		Digest:      indexDigest,
		Annotations: map[string]string{zarfLayerTitleAnnotation: "images/index.json"},
	})

	// Write images/oci-layout
	ociLayoutData := []byte(`{"imageLayoutVersion":"1.0.0"}`)
	ociLayoutDigest := writeBlobBytes(t, blobDir, ociLayoutData)
	layers = append(layers, ociDescriptor{
		Digest:      ociLayoutDigest,
		Annotations: map[string]string{zarfLayerTitleAnnotation: "images/oci-layout"},
	})

	return layers, blobDir
}

func TestImageBlobsToExclude_NoImagesExcluded(t *testing.T) {
	// When all components are kept, no image blobs should be excluded
	pkg := v1alpha1.ZarfPackage{
		Components: []v1alpha1.ZarfComponent{
			{Name: "a", Images: []string{"img1"}},
			{Name: "b", Images: []string{"img2"}},
		},
	}
	keepAll := map[string]bool{"a": true, "b": true}

	result, err := imageBlobsToExclude(context.Background(), iostreams.IOStreams{}, "unused", nil, pkg, keepAll)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestImageBlobsToExclude_ComponentsWithNoImages(t *testing.T) {
	// When excluded components have no images, nothing to exclude
	pkg := v1alpha1.ZarfPackage{
		Components: []v1alpha1.ZarfComponent{
			{Name: "core", Images: []string{"img1"}},
			{Name: "optional"}, // no images
		},
	}
	keep := map[string]bool{"core": true}

	result, err := imageBlobsToExclude(context.Background(), iostreams.IOStreams{}, "unused", nil, pkg, keep)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestImageBlobsToExclude_NoImagesIndex(t *testing.T) {
	// When there's no images/index.json layer, return nil gracefully
	pkg := v1alpha1.ZarfPackage{
		Components: []v1alpha1.ZarfComponent{
			{Name: "core"},
			{Name: "optional", Images: []string{"img1"}},
		},
	}
	keep := map[string]bool{"core": true}
	layers := []ociDescriptor{
		{Digest: "sha256:abc123", Annotations: map[string]string{zarfLayerTitleAnnotation: "zarf.yaml"}},
	}

	result, err := imageBlobsToExclude(context.Background(), iostreams.IOStreams{}, t.TempDir(), layers, pkg, keep)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestImageBlobsToExclude_ExcludesCorrectBlobs(t *testing.T) {
	keptImageLayer := godigest.FromString("kept-layer")
	keptImageConfig := godigest.FromString("kept-config")
	keptImageManifest := godigest.FromString("kept-manifest")

	excludedImageLayer := godigest.FromString("excluded-layer")
	excludedImageConfig := godigest.FromString("excluded-config")
	excludedImageManifest := godigest.FromString("excluded-manifest")

	imageIndex := ocispec.Index{
		Manifests: []ocispec.Descriptor{
			{
				Digest:      keptImageManifest,
				Annotations: map[string]string{ocispec.AnnotationBaseImageName: "kept-image:v1"},
			},
			{
				Digest:      excludedImageManifest,
				Annotations: map[string]string{ocispec.AnnotationBaseImageName: "excluded-image:v1"},
			},
		},
	}
	imageManifests := map[string]ocispec.Manifest{
		keptImageManifest.String(): {
			Config: ocispec.Descriptor{Digest: keptImageConfig},
			Layers: []ocispec.Descriptor{{Digest: keptImageLayer}},
		},
		excludedImageManifest.String(): {
			Config: ocispec.Descriptor{Digest: excludedImageConfig},
			Layers: []ocispec.Descriptor{{Digest: excludedImageLayer}},
		},
	}

	layers, blobDir := setupImageBlobsTestFixture(t, imageIndex, imageManifests)

	pkg := v1alpha1.ZarfPackage{
		Components: []v1alpha1.ZarfComponent{
			{Name: "core", Images: []string{"kept-image:v1"}},
			{Name: "optional", Images: []string{"excluded-image:v1"}},
		},
	}
	keep := map[string]bool{"core": true}

	result, err := imageBlobsToExclude(context.Background(), iostreams.IOStreams{}, blobDir, layers, pkg, keep)
	require.NoError(t, err)

	// Excluded image blobs should be in the result
	assert.True(t, result[excludedImageLayer.String()], "excluded image layer")
	assert.True(t, result[excludedImageConfig.String()], "excluded image config")
	assert.True(t, result[excludedImageManifest.String()], "excluded image manifest")

	// Kept image blobs should NOT be in the result
	assert.False(t, result[keptImageLayer.String()], "kept image layer should not be excluded")
	assert.False(t, result[keptImageConfig.String()], "kept image config should not be excluded")
	assert.False(t, result[keptImageManifest.String()], "kept image manifest should not be excluded")
}

func TestImageBlobsToExclude_SharedLayersPreserved(t *testing.T) {
	sharedLayer := godigest.FromString("shared-layer")
	keptConfig := godigest.FromString("kept-config")
	excludedConfig := godigest.FromString("excluded-config")
	keptManifest := godigest.FromString("kept-manifest")
	excludedManifest := godigest.FromString("excluded-manifest")

	imageIndex := ocispec.Index{
		Manifests: []ocispec.Descriptor{
			{
				Digest:      keptManifest,
				Annotations: map[string]string{ocispec.AnnotationBaseImageName: "kept-image:v1"},
			},
			{
				Digest:      excludedManifest,
				Annotations: map[string]string{ocispec.AnnotationBaseImageName: "excluded-image:v1"},
			},
		},
	}
	imageManifests := map[string]ocispec.Manifest{
		keptManifest.String(): {
			Config: ocispec.Descriptor{Digest: keptConfig},
			Layers: []ocispec.Descriptor{{Digest: sharedLayer}}, // shared!
		},
		excludedManifest.String(): {
			Config: ocispec.Descriptor{Digest: excludedConfig},
			Layers: []ocispec.Descriptor{{Digest: sharedLayer}}, // shared!
		},
	}

	layers, blobDir := setupImageBlobsTestFixture(t, imageIndex, imageManifests)

	pkg := v1alpha1.ZarfPackage{
		Components: []v1alpha1.ZarfComponent{
			{Name: "core", Images: []string{"kept-image:v1"}},
			{Name: "optional", Images: []string{"excluded-image:v1"}},
		},
	}
	keep := map[string]bool{"core": true}

	result, err := imageBlobsToExclude(context.Background(), iostreams.IOStreams{}, blobDir, layers, pkg, keep)
	require.NoError(t, err)

	// Shared layer should NOT be excluded
	assert.False(t, result[sharedLayer.String()], "shared layer should be preserved")

	// Excluded-only blobs should be excluded
	assert.True(t, result[excludedConfig.String()], "excluded config should be removed")
	assert.True(t, result[excludedManifest.String()], "excluded manifest should be removed")
}

func TestImageBlobsToExclude_AllImagesExcluded(t *testing.T) {
	imageLayer := godigest.FromString("image-layer")
	imageConfig := godigest.FromString("image-config")
	imageManifestDigest := godigest.FromString("image-manifest")

	imageIndex := ocispec.Index{
		Manifests: []ocispec.Descriptor{
			{
				Digest:      imageManifestDigest,
				Annotations: map[string]string{ocispec.AnnotationBaseImageName: "some-image:v1"},
			},
		},
	}
	imageManifests := map[string]ocispec.Manifest{
		imageManifestDigest.String(): {
			Config: ocispec.Descriptor{Digest: imageConfig},
			Layers: []ocispec.Descriptor{{Digest: imageLayer}},
		},
	}

	layers, blobDir := setupImageBlobsTestFixture(t, imageIndex, imageManifests)

	// Only the excluded component has images; the kept component has none
	pkg := v1alpha1.ZarfPackage{
		Components: []v1alpha1.ZarfComponent{
			{Name: "core"},
			{Name: "optional", Images: []string{"some-image:v1"}},
		},
	}
	keep := map[string]bool{"core": true}

	result, err := imageBlobsToExclude(context.Background(), iostreams.IOStreams{}, blobDir, layers, pkg, keep)
	require.NoError(t, err)

	// Image blobs should be excluded
	assert.True(t, result[imageLayer.String()])
	assert.True(t, result[imageConfig.String()])
	assert.True(t, result[imageManifestDigest.String()])

	// When ALL images are excluded, images/index.json and images/oci-layout should also be excluded
	indexExcluded := false
	ociLayoutExcluded := false
	for _, l := range layers {
		title := l.Annotations[zarfLayerTitleAnnotation]
		if title == "images/index.json" && result[l.Digest] {
			indexExcluded = true
		}
		if title == "images/oci-layout" && result[l.Digest] {
			ociLayoutExcluded = true
		}
	}
	assert.True(t, indexExcluded, "images/index.json should be excluded when all images are removed")
	assert.True(t, ociLayoutExcluded, "images/oci-layout should be excluded when all images are removed")
}
