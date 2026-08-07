// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package pusher

import (
	"bytes"
	"context"
	"testing"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/goccy/go-yaml"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
)

func TestRemotePusherSelectLayersFiltersOptionalComponents(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	required := true
	pkgBytes, err := yaml.Marshal(v1alpha1.ZarfPackage{
		Metadata: v1alpha1.ZarfMetadata{Name: "test"},
		Components: []v1alpha1.ZarfComponent{
			{Name: "required", Required: &required},
			{Name: "optional"},
		},
	})
	require.NoError(t, err)
	zarfYAMLDesc := pushPackageFile(t, ctx, store, layout.ZarfYAML, pkgBytes)
	requiredDesc := pushPackageFile(t, ctx, store, "components/required.tar", []byte("required"))
	optionalDesc := pushPackageFile(t, ctx, store, "components/optional.tar", []byte("optional"))
	manifest := &oci.Manifest{Manifest: ocispec.Manifest{
		Layers: []ocispec.Descriptor{zarfYAMLDesc, requiredDesc, optionalDesc},
	}}
	pusher := RemotePusher{
		pkg: types.Package{Name: "test"},
		cfg: Config{PkgRootManifest: manifest},
	}

	layers, err := pusher.selectLayers(ctx, store)
	require.NoError(t, err)
	require.ElementsMatch(t,
		[]digest.Digest{zarfYAMLDesc.Digest, requiredDesc.Digest},
		descriptorDigests(layers),
	)
}

func pushPackageFile(t *testing.T, ctx context.Context, store content.Storage, path string, data []byte) ocispec.Descriptor {
	t.Helper()
	desc := content.NewDescriptorFromBytes(layout.ZarfLayerMediaTypeBlob, data)
	desc.Annotations = map[string]string{ocispec.AnnotationTitle: path}
	require.NoError(t, store.Push(ctx, desc, bytes.NewReader(data)))
	return desc
}

func descriptorDigests(descriptors []ocispec.Descriptor) []digest.Digest {
	digests := make([]digest.Digest, 0, len(descriptors))
	for _, desc := range descriptors {
		digests = append(digests, desc.Digest)
	}
	return digests
}
