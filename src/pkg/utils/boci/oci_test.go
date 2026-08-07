// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package boci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	goyaml "github.com/goccy/go-yaml"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
)

func TestSelectPackageContentFiltersOptionalComponents(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	required := true
	pkg := v1alpha1.ZarfPackage{
		Metadata: v1alpha1.ZarfMetadata{Name: "test-package"},
		Components: []v1alpha1.ZarfComponent{
			{Name: "required", Required: &required},
			{Name: "optional"},
		},
	}
	pkgBytes, err := goyaml.Marshal(pkg)
	require.NoError(t, err)

	zarfYAMLDesc := pushTestPackageFile(t, ctx, store, layout.ZarfYAML, layout.ZarfLayerMediaTypeBlob, pkgBytes)
	checksumsDesc := pushTestPackageFile(t, ctx, store, layout.Checksums, layout.ZarfLayerMediaTypeBlob, []byte("checksums"))
	requiredDesc := pushTestPackageFile(t, ctx, store, "components/required.tar", layout.ZarfLayerMediaTypeBlob, []byte("required"))
	optionalDesc := pushTestPackageFile(t, ctx, store, "components/optional.tar", layout.ZarfLayerMediaTypeBlob, []byte("optional"))
	manifest := &oci.Manifest{Manifest: ocispec.Manifest{
		Layers: []ocispec.Descriptor{zarfYAMLDesc, checksumsDesc, requiredDesc, optionalDesc},
	}}

	t.Run("required only", func(t *testing.T) {
		layers, err := SelectPackageContent(ctx, manifest, store, nil)
		require.NoError(t, err)
		require.ElementsMatch(t,
			[]digest.Digest{zarfYAMLDesc.Digest, checksumsDesc.Digest, requiredDesc.Digest},
			descriptorDigests(layers),
		)
	})

	t.Run("selected optional", func(t *testing.T) {
		layers, err := SelectPackageContent(ctx, manifest, store, []string{"optional"})
		require.NoError(t, err)
		require.ElementsMatch(t,
			[]digest.Digest{zarfYAMLDesc.Digest, checksumsDesc.Digest, requiredDesc.Digest, optionalDesc.Digest},
			descriptorDigests(layers),
		)
	})

	t.Run("missing optional", func(t *testing.T) {
		_, err := SelectPackageContent(ctx, manifest, store, []string{"missing"})
		require.ErrorContains(t, err, "no compatible components found")
	})
}

func TestSelectPackageContentUsesZarfImageSelection(t *testing.T) {
	const imageRef = "ghcr.io/example/app:v1"
	ctx := context.Background()
	store := memory.New()
	required := true
	pkg := v1alpha1.ZarfPackage{
		Metadata: v1alpha1.ZarfMetadata{Name: "image-package"},
		Components: []v1alpha1.ZarfComponent{{
			Name:     "required",
			Required: &required,
			ImageArchives: []v1alpha1.ImageArchive{{
				Path:   "images.tar",
				Images: []string{imageRef},
			}},
		}},
	}
	pkgBytes, err := goyaml.Marshal(pkg)
	require.NoError(t, err)

	configDesc := pushTestBlob(t, ctx, store, ocispec.MediaTypeImageConfig, []byte(`{"architecture":"amd64","os":"linux"}`))
	layerDesc := pushTestBlob(t, ctx, store, ocispec.MediaTypeImageLayer, []byte("image layer"))
	imageManifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    []ocispec.Descriptor{layerDesc},
	}
	imageManifestDesc := pushTestJSON(t, ctx, store, ocispec.MediaTypeImageManifest, imageManifest)
	indexEntry := imageManifestDesc
	indexEntry.Annotations = map[string]string{ocispec.AnnotationBaseImageName: imageRef}
	imageIndex := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{indexEntry},
	}
	indexBytes, err := json.Marshal(imageIndex)
	require.NoError(t, err)

	zarfYAMLDesc := pushTestPackageFile(t, ctx, store, layout.ZarfYAML, layout.ZarfLayerMediaTypeBlob, pkgBytes)
	componentDesc := pushTestPackageFile(t, ctx, store, "components/required.tar", layout.ZarfLayerMediaTypeBlob, []byte("required"))
	indexDesc := pushTestPackageFile(t, ctx, store, layout.IndexPath, layout.ZarfLayerMediaTypeBlob, indexBytes)
	ociLayoutDesc := pushTestPackageFile(t, ctx, store, layout.OCILayoutPath, layout.ZarfLayerMediaTypeBlob, []byte(`{"imageLayoutVersion":"1.0.0"}`))
	imageManifestLayer := packageBlobDescriptor(imageManifestDesc)
	configLayer := packageBlobDescriptor(configDesc)
	imageLayer := packageBlobDescriptor(layerDesc)
	manifest := &oci.Manifest{Manifest: ocispec.Manifest{Layers: []ocispec.Descriptor{
		zarfYAMLDesc, componentDesc, indexDesc, ociLayoutDesc, imageManifestLayer, configLayer, imageLayer,
	}}}

	layers, err := SelectPackageContent(ctx, manifest, store, nil)
	require.NoError(t, err)
	require.ElementsMatch(t,
		[]digest.Digest{
			zarfYAMLDesc.Digest,
			componentDesc.Digest,
			indexDesc.Digest,
			ociLayoutDesc.Digest,
			imageManifestLayer.Digest,
			configLayer.Digest,
			imageLayer.Digest,
		},
		descriptorDigests(layers),
	)
}

func TestSelectPackageContentIncludesMultiPlatformImage(t *testing.T) {
	const imageRef = "ghcr.io/example/multi:v1"
	ctx := context.Background()
	store := memory.New()
	required := true
	pkg := v1alpha1.ZarfPackage{
		Metadata: v1alpha1.ZarfMetadata{Name: "multi-platform-package"},
		Components: []v1alpha1.ZarfComponent{{
			Name:     "required",
			Required: &required,
			Images:   []string{imageRef},
		}},
	}
	pkgBytes, err := goyaml.Marshal(pkg)
	require.NoError(t, err)

	configDesc := pushTestBlob(t, ctx, store, ocispec.MediaTypeImageConfig, []byte(`{"architecture":"multi","os":"linux"}`))
	layerDesc := pushTestBlob(t, ctx, store, ocispec.MediaTypeImageLayer, []byte("shared layer"))
	imageManifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    []ocispec.Descriptor{layerDesc},
		Annotations: map[string]string{
			"test.architecture": "amd64",
		},
	}
	amdManifestDesc := pushTestJSON(t, ctx, store, ocispec.MediaTypeImageManifest, imageManifest)
	imageManifest.Annotations["test.architecture"] = "arm64"
	armManifestDesc := pushTestJSON(t, ctx, store, ocispec.MediaTypeImageManifest, imageManifest)
	nestedIndex := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{amdManifestDesc, armManifestDesc},
	}
	nestedIndexDesc := pushTestJSON(t, ctx, store, ocispec.MediaTypeImageIndex, nestedIndex)
	topIndex := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{{
			MediaType: ocispec.MediaTypeImageIndex,
			Digest:    nestedIndexDesc.Digest,
			Size:      nestedIndexDesc.Size,
			Annotations: map[string]string{
				ocispec.AnnotationBaseImageName: imageRef,
			},
		}},
	}
	topIndexBytes, err := json.Marshal(topIndex)
	require.NoError(t, err)

	zarfYAMLDesc := pushTestPackageFile(t, ctx, store, layout.ZarfYAML, layout.ZarfLayerMediaTypeBlob, pkgBytes)
	componentDesc := pushTestPackageFile(t, ctx, store, "components/required.tar", layout.ZarfLayerMediaTypeBlob, []byte("required"))
	topIndexDesc := pushTestPackageFile(t, ctx, store, layout.IndexPath, layout.ZarfLayerMediaTypeBlob, topIndexBytes)
	ociLayoutDesc := pushTestPackageFile(t, ctx, store, layout.OCILayoutPath, layout.ZarfLayerMediaTypeBlob, []byte(`{"imageLayoutVersion":"1.0.0"}`))
	manifest := &oci.Manifest{Manifest: ocispec.Manifest{Layers: []ocispec.Descriptor{
		zarfYAMLDesc,
		componentDesc,
		topIndexDesc,
		ociLayoutDesc,
		packageBlobDescriptor(nestedIndexDesc),
		packageBlobDescriptor(amdManifestDesc),
		packageBlobDescriptor(armManifestDesc),
		packageBlobDescriptor(configDesc),
		packageBlobDescriptor(layerDesc),
	}}}

	layers, err := SelectPackageContent(ctx, manifest, store, nil)
	require.NoError(t, err)
	require.ElementsMatch(t,
		[]digest.Digest{
			zarfYAMLDesc.Digest,
			componentDesc.Digest,
			topIndexDesc.Digest,
			ociLayoutDesc.Digest,
			nestedIndexDesc.Digest,
			amdManifestDesc.Digest,
			armManifestDesc.Digest,
			configDesc.Digest,
			layerDesc.Digest,
		},
		descriptorDigests(layers),
	)
}

func TestSelectPackageContentExcludesSkeletonImages(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	required := true
	pkg := v1alpha1.ZarfPackage{
		Metadata: v1alpha1.ZarfMetadata{Name: "skeleton-package"},
		Build:    v1alpha1.ZarfBuildData{Architecture: v1alpha1.SkeletonArch},
		Components: []v1alpha1.ZarfComponent{{
			Name:     "required",
			Required: &required,
			Images:   []string{"ghcr.io/example/app:v1"},
		}},
	}
	pkgBytes, err := goyaml.Marshal(pkg)
	require.NoError(t, err)
	zarfYAMLDesc := pushTestPackageFile(t, ctx, store, layout.ZarfYAML, layout.ZarfLayerMediaTypeBlob, pkgBytes)
	componentDesc := pushTestPackageFile(t, ctx, store, "components/required.tar", layout.ZarfLayerMediaTypeBlob, []byte("required"))
	manifest := &oci.Manifest{Manifest: ocispec.Manifest{Layers: []ocispec.Descriptor{zarfYAMLDesc, componentDesc}}}

	layers, err := SelectPackageContent(ctx, manifest, store, nil)
	require.NoError(t, err)
	require.ElementsMatch(t,
		[]digest.Digest{zarfYAMLDesc.Digest, componentDesc.Digest},
		descriptorDigests(layers),
	)
}

func TestSelectPackageContentOmitsEmptyComponentDescriptor(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	required := true
	pkg := v1alpha1.ZarfPackage{
		Metadata:   v1alpha1.ZarfMetadata{Name: "missing-component"},
		Components: []v1alpha1.ZarfComponent{{Name: "required", Required: &required}},
	}
	pkgBytes, err := goyaml.Marshal(pkg)
	require.NoError(t, err)
	zarfYAMLDesc := pushTestPackageFile(t, ctx, store, layout.ZarfYAML, layout.ZarfLayerMediaTypeBlob, pkgBytes)
	manifest := &oci.Manifest{Manifest: ocispec.Manifest{Layers: []ocispec.Descriptor{zarfYAMLDesc}}}

	layers, err := SelectPackageContent(ctx, manifest, store, nil)
	require.NoError(t, err)
	require.Equal(t, []digest.Digest{zarfYAMLDesc.Digest}, descriptorDigests(layers))
}

func TestSelectPackageContentIncludesManifestConfig(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	pkg := v1alpha1.ZarfPackage{Metadata: v1alpha1.ZarfMetadata{Name: "configured-package"}}
	pkgBytes, err := goyaml.Marshal(pkg)
	require.NoError(t, err)
	zarfYAMLDesc := pushTestPackageFile(t, ctx, store, layout.ZarfYAML, layout.ZarfLayerMediaTypeBlob, pkgBytes)
	configDesc := pushTestBlob(t, ctx, store, ocispec.MediaTypeImageConfig, []byte(`{"architecture":"amd64"}`))
	manifest := &oci.Manifest{Manifest: ocispec.Manifest{
		Config: configDesc,
		Layers: []ocispec.Descriptor{zarfYAMLDesc},
	}}

	layers, err := SelectPackageContent(ctx, manifest, store, nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []ocispec.Descriptor{zarfYAMLDesc, configDesc}, layers)
}

func TestSelectPackageContentPreservesDistinctPathsForSharedContent(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	required := true
	pkg := v1alpha1.ZarfPackage{
		Metadata: v1alpha1.ZarfMetadata{Name: "shared-content"},
		Components: []v1alpha1.ZarfComponent{
			{Name: "first", Required: &required},
			{Name: "second", Required: &required},
		},
	}
	pkgBytes, err := goyaml.Marshal(pkg)
	require.NoError(t, err)
	zarfYAMLDesc := pushTestPackageFile(t, ctx, store, layout.ZarfYAML, layout.ZarfLayerMediaTypeBlob, pkgBytes)
	firstDesc := pushTestPackageFile(t, ctx, store, "components/first.tar", layout.ZarfLayerMediaTypeBlob, []byte("shared"))
	secondDesc := firstDesc
	secondDesc.Annotations = map[string]string{ocispec.AnnotationTitle: "components/second.tar"}
	manifest := &oci.Manifest{Manifest: ocispec.Manifest{Layers: []ocispec.Descriptor{zarfYAMLDesc, firstDesc, secondDesc}}}

	layers, err := SelectPackageContent(ctx, manifest, store, nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []ocispec.Descriptor{zarfYAMLDesc, firstDesc, secondDesc}, layers)
}

func TestSelectBundledPackageContentIncludesManifestAndPreservesDistinctPaths(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	required := true
	pkg := v1alpha1.ZarfPackage{
		Metadata: v1alpha1.ZarfMetadata{Name: "nested-package"},
		Components: []v1alpha1.ZarfComponent{
			{Name: "first", Required: &required},
			{Name: "second", Required: &required},
		},
	}
	pkgBytes, err := goyaml.Marshal(pkg)
	require.NoError(t, err)
	zarfYAMLDesc := pushTestPackageFile(t, ctx, store, layout.ZarfYAML, layout.ZarfLayerMediaTypeBlob, pkgBytes)
	firstDesc := pushTestPackageFile(t, ctx, store, "components/first.tar", layout.ZarfLayerMediaTypeBlob, []byte("shared"))
	secondDesc := firstDesc
	secondDesc.Annotations = map[string]string{ocispec.AnnotationTitle: "components/second.tar"}
	configDesc := pushTestBlob(t, ctx, store, ocispec.MediaTypeImageConfig, []byte(`{"architecture":"amd64"}`))
	pkgManifest := oci.Manifest{Manifest: ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Config:    configDesc,
		Layers:    []ocispec.Descriptor{zarfYAMLDesc, firstDesc, secondDesc},
	}}
	pkgManifestDesc := pushTestJSON(t, ctx, store, layout.ZarfLayerMediaTypeBlob, pkgManifest)
	bundleManifest := &oci.Manifest{Manifest: ocispec.Manifest{Layers: []ocispec.Descriptor{pkgManifestDesc}}}

	contentDescs, err := SelectBundledPackageContent(ctx, bundleManifest, store, pkgManifestDesc.Digest, nil)
	require.NoError(t, err)
	require.ElementsMatch(t,
		[]ocispec.Descriptor{zarfYAMLDesc, firstDesc, secondDesc, configDesc, pkgManifestDesc},
		contentDescs,
	)
}

func TestSelectBundledPackageContentOmitsMissingManifestConfig(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	pkgBytes, err := goyaml.Marshal(v1alpha1.ZarfPackage{
		Metadata: v1alpha1.ZarfMetadata{Name: "legacy-package"},
	})
	require.NoError(t, err)
	zarfYAMLDesc := pushTestPackageFile(t, ctx, store, layout.ZarfYAML, layout.ZarfLayerMediaTypeBlob, pkgBytes)
	missingConfigDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageConfig, []byte("missing config"))
	pkgManifest := oci.Manifest{Manifest: ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Config:    missingConfigDesc,
		Layers:    []ocispec.Descriptor{zarfYAMLDesc},
	}}
	pkgManifestDesc := pushTestJSON(t, ctx, store, layout.ZarfLayerMediaTypeBlob, pkgManifest)
	bundleManifest := &oci.Manifest{Manifest: ocispec.Manifest{Layers: []ocispec.Descriptor{pkgManifestDesc}}}

	contentDescs, err := SelectBundledPackageContent(ctx, bundleManifest, store, pkgManifestDesc.Digest, nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []ocispec.Descriptor{zarfYAMLDesc, pkgManifestDesc}, contentDescs)
}

func TestSelectBundledPackageContentPropagatesManifestConfigProbeErrors(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	pkgBytes, err := goyaml.Marshal(v1alpha1.ZarfPackage{
		Metadata: v1alpha1.ZarfMetadata{Name: "unavailable-package"},
	})
	require.NoError(t, err)
	zarfYAMLDesc := pushTestPackageFile(t, ctx, store, layout.ZarfYAML, layout.ZarfLayerMediaTypeBlob, pkgBytes)
	configDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageConfig, []byte("unavailable config"))
	pkgManifest := oci.Manifest{Manifest: ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Config:    configDesc,
		Layers:    []ocispec.Descriptor{zarfYAMLDesc},
	}}
	pkgManifestDesc := pushTestJSON(t, ctx, store, layout.ZarfLayerMediaTypeBlob, pkgManifest)
	bundleManifest := &oci.Manifest{Manifest: ocispec.Manifest{Layers: []ocispec.Descriptor{pkgManifestDesc}}}
	fetcher := descriptorErrorFetcher{
		Fetcher: store,
		digest:  configDesc.Digest,
		err:     errors.New("registry unavailable"),
	}

	_, err = SelectBundledPackageContent(ctx, bundleManifest, fetcher, pkgManifestDesc.Digest, nil)
	require.ErrorContains(t, err, "registry unavailable")
}

func TestSelectBundledPackageContentRejectsMissingManifest(t *testing.T) {
	_, err := SelectBundledPackageContent(
		context.Background(),
		&oci.Manifest{},
		memory.New(),
		digest.FromString("missing"),
		nil,
	)
	require.ErrorContains(t, err, "does not exist in bundle")
}

func TestMaterializePackagePathsRejectsTraversal(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	desc := pushTestPackageFile(t, ctx, store, "../outside", layout.ZarfLayerMediaTypeBlob, []byte("unsafe"))

	err := MaterializePackagePaths(ctx, store, t.TempDir(), []ocispec.Descriptor{desc})
	require.EqualError(t, err, `invalid package path "../outside"`)
}

func TestMaterializePackagePathsWritesEachPathForSharedContent(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	firstDesc := pushTestPackageFile(t, ctx, store, "components/first.tar", layout.ZarfLayerMediaTypeBlob, []byte("shared"))
	secondDesc := firstDesc
	secondDesc.Annotations = map[string]string{ocispec.AnnotationTitle: "components/second.tar"}
	destination := t.TempDir()

	require.NoError(t, MaterializePackagePaths(ctx, store, destination, []ocispec.Descriptor{firstDesc, secondDesc}))
	for _, path := range []string{"components/first.tar", "components/second.tar"} {
		actual, err := os.ReadFile(filepath.Join(destination, path))
		require.NoError(t, err)
		require.Equal(t, []byte("shared"), actual)
	}
}

func TestCreateCopyOptsImageIndexes(t *testing.T) {
	ctx := context.Background()
	store := memory.New()

	otherArch := "arm64"
	if config.GetArch() == otherArch {
		otherArch = "amd64"
	}
	targetManifest := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromString("target manifest"),
		Size:      1,
		Platform:  &ocispec.Platform{Architecture: config.GetArch(), OS: "linux"},
	}
	otherManifest := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromString("other manifest"),
		Size:      1,
		Platform:  &ocispec.Platform{Architecture: otherArch, OS: "linux"},
	}
	index := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{
			targetManifest,
			otherManifest,
		},
	}
	indexDesc := pushTestJSON(t, ctx, store, ocispec.MediaTypeImageIndex, index)

	t.Run("bundle root selects target architecture", func(t *testing.T) {
		findSuccessors := CreateCopyOpts([]ocispec.Descriptor{targetManifest}, 1).FindSuccessors
		successors, err := findSuccessors(ctx, store, indexDesc)
		require.NoError(t, err)
		require.Equal(t, []ocispec.Descriptor{targetManifest}, successors)
	})

	t.Run("unannotated embedded image index preserves all manifests", func(t *testing.T) {
		layersToPull := []ocispec.Descriptor{indexDesc, targetManifest, otherManifest}
		findSuccessors := CreateCopyOpts(layersToPull, 1).FindSuccessors
		successors, err := findSuccessors(ctx, store, indexDesc)
		require.NoError(t, err)
		require.ElementsMatch(t, []ocispec.Descriptor{targetManifest, otherManifest}, successors)
	})
}

func pushTestBlob(t *testing.T, ctx context.Context, store content.Storage, mediaType string, data []byte) ocispec.Descriptor {
	t.Helper()
	desc := content.NewDescriptorFromBytes(mediaType, data)
	require.NoError(t, store.Push(ctx, desc, bytes.NewReader(data)))
	return desc
}

func pushTestPackageFile(t *testing.T, ctx context.Context, store content.Storage, path, mediaType string, data []byte) ocispec.Descriptor {
	t.Helper()
	desc := content.NewDescriptorFromBytes(mediaType, data)
	desc.Annotations = map[string]string{ocispec.AnnotationTitle: path}
	require.NoError(t, store.Push(ctx, desc, bytes.NewReader(data)))
	return desc
}

func packageBlobDescriptor(desc ocispec.Descriptor) ocispec.Descriptor {
	desc.Annotations = map[string]string{ocispec.AnnotationTitle: filepath.Join(layout.ImagesBlobsDir, desc.Digest.Encoded())}
	return desc
}

func pushTestJSON(t *testing.T, ctx context.Context, store content.Storage, mediaType string, value any) ocispec.Descriptor {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return pushTestBlob(t, ctx, store, mediaType, data)
}

func descriptorDigests(descriptors []ocispec.Descriptor) []digest.Digest {
	digests := make([]digest.Digest, 0, len(descriptors))
	for _, desc := range descriptors {
		digests = append(digests, desc.Digest)
	}
	return digests
}

type descriptorErrorFetcher struct {
	content.Fetcher
	digest digest.Digest
	err    error
}

func (f descriptorErrorFetcher) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	if desc.Digest == f.digest {
		return nil, f.err
	}
	return f.Fetcher.Fetch(ctx, desc)
}
