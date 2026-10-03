// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package pusher

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	legacyconfig "github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	legacytypes "github.com/defenseunicorns/uds-cli/pkg/legacy/types"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/utils/boci"
	goyaml "github.com/goccy/go-yaml"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	zarfoci "github.com/zarf-dev/zarf/src/pkg/oci"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
	"oras.land/oras-go/v2/content"
)

func TestRemoteToRemoteCopiesSelectedLayersAndConfig(t *testing.T) {
	srcServer := httptest.NewServer(registry.New())
	t.Cleanup(srcServer.Close)
	dstServer := httptest.NewServer(registry.New())
	t.Cleanup(dstServer.Close)
	platform := ocispec.Platform{Architecture: "amd64", OS: zarfoci.MultiOS}
	options := zoci.RemoteClientOptions{RemoteOptions: zarfTypes.RemoteOptions{PlainHTTP: true}}
	src, err := zoci.NewRemoteWithOptions(t.Context(), strings.TrimPrefix(srcServer.URL, "http://")+"/test/package:v1", platform, options)
	require.NoError(t, err)
	dst, err := zoci.NewRemoteWithOptions(t.Context(), strings.TrimPrefix(dstServer.URL, "http://")+"/test/package:v1", platform, options)
	require.NoError(t, err)
	push := func(data string) ocispec.Descriptor {
		t.Helper()
		desc := content.NewDescriptorFromBytes("application/vnd.zarf.layer.v1.blob", []byte(data))
		require.NoError(t, src.Repo().Push(t.Context(), desc, bytes.NewReader([]byte(data))))
		return desc
	}
	included := push("included")
	excluded := push("excluded")
	config := push("config")
	p := RemotePusher{cfg: Config{
		PkgRootManifest: &zarfoci.Manifest{Manifest: ocispec.Manifest{Config: config}},
		RemoteSrc:       *src,
		RemoteDst:       *dst,
	}}
	require.NoError(t, p.remoteToRemote([]ocispec.Descriptor{included}))

	exists, err := dst.Repo().Exists(t.Context(), included)
	require.NoError(t, err)
	require.True(t, exists)
	exists, err = dst.Repo().Exists(t.Context(), config)
	require.NoError(t, err)
	require.True(t, exists)
	exists, err = dst.Repo().Exists(t.Context(), excluded)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestRemotePusherPublishesPackageWithImageGraph(t *testing.T) {
	ctx := t.Context()
	srcServer := httptest.NewServer(registry.New())
	t.Cleanup(srcServer.Close)
	dstServer := httptest.NewServer(registry.New())
	t.Cleanup(dstServer.Close)
	platform := ocispec.Platform{Architecture: "amd64", OS: zarfoci.MultiOS}
	options := zoci.RemoteClientOptions{RemoteOptions: zarfTypes.RemoteOptions{PlainHTTP: true}}
	src, err := zoci.NewRemoteWithOptions(ctx, strings.TrimPrefix(srcServer.URL, "http://")+"/test/package:v1", platform, options)
	require.NoError(t, err)
	dst, err := zoci.NewRemoteWithOptions(ctx, strings.TrimPrefix(dstServer.URL, "http://")+"/test/bundle:v1", platform, options)
	require.NoError(t, err)

	push := func(mediaType string, data []byte) ocispec.Descriptor {
		t.Helper()
		desc := content.NewDescriptorFromBytes(mediaType, data)
		require.NoError(t, src.Repo().Push(ctx, desc, bytes.NewReader(data)))
		return desc
	}
	pushJSON := func(mediaType string, value any) ocispec.Descriptor {
		t.Helper()
		data, err := json.Marshal(value)
		require.NoError(t, err)
		return push(mediaType, data)
	}
	withTitle := func(desc ocispec.Descriptor, title string) ocispec.Descriptor {
		desc.Annotations = map[string]string{ocispec.AnnotationTitle: title}
		return desc
	}

	imageConfig := push(ocispec.MediaTypeImageConfig, []byte(`{"architecture":"amd64","os":"linux"}`))
	imageLayer := push(ocispec.MediaTypeImageLayer, []byte("image layer"))
	imageManifest := pushJSON(ocispec.MediaTypeImageManifest, ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Config:    imageConfig,
		Layers:    []ocispec.Descriptor{imageLayer},
	})
	imageIndex := pushJSON(ocispec.MediaTypeImageIndex, ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Manifests: []ocispec.Descriptor{{
			MediaType: imageManifest.MediaType,
			Digest:    imageManifest.Digest,
			Size:      imageManifest.Size,
			Platform:  &ocispec.Platform{Architecture: "amd64", OS: "linux"},
		}},
	})
	const imageName = "docker.io/library/test:v1"
	const sharedImageName = "docker.io/library/shared:v1"
	sharedImageIndex := pushJSON(ocispec.MediaTypeImageIndex, ocispec.Index{
		Versioned:   specs.Versioned{SchemaVersion: 2},
		Annotations: map[string]string{"image": "shared"},
		Manifests: []ocispec.Descriptor{{
			MediaType: imageManifest.MediaType,
			Digest:    imageManifest.Digest,
			Size:      imageManifest.Size,
			Platform:  &ocispec.Platform{Architecture: "amd64", OS: "linux"},
		}},
	})
	packageImageIndex := pushJSON(ocispec.MediaTypeImageIndex, ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Manifests: []ocispec.Descriptor{
			{MediaType: imageIndex.MediaType, Digest: imageIndex.Digest, Size: imageIndex.Size, Annotations: map[string]string{ocispec.AnnotationBaseImageName: imageName}},
			{MediaType: sharedImageIndex.MediaType, Digest: sharedImageIndex.Digest, Size: sharedImageIndex.Size, Annotations: map[string]string{ocispec.AnnotationBaseImageName: sharedImageName}},
		},
	})
	zarfYAML := push(layout.ZarfLayerMediaTypeBlob, []byte("kind: ZarfPackageConfig\nmetadata:\n  name: test\n  version: v1\ncomponents:\n  - name: main\n    required: true\n    images:\n      - "+imageName+"\n      - "+sharedImageName+"\n"))
	packageConfig := push(layout.ZarfLayerMediaTypeBlob, []byte("package config"))
	packageRoot := &zarfoci.Manifest{Manifest: ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Config:    packageConfig,
		Layers: []ocispec.Descriptor{
			withTitle(zarfYAML, layout.ZarfYAML),
			withTitle(push(layout.ZarfLayerMediaTypeBlob, []byte("component")), "components/main.tar"),
			withTitle(packageImageIndex, layout.IndexPath),
			withTitle(push(layout.ZarfLayerMediaTypeBlob, []byte(`{"imageLayoutVersion":"1.0.0"}`)), layout.OCILayoutPath),
			withTitle(imageIndex, layout.ImagesBlobsDir+"/"+imageIndex.Digest.Encoded()),
			withTitle(sharedImageIndex, layout.ImagesBlobsDir+"/"+sharedImageIndex.Digest.Encoded()),
			withTitle(imageManifest, layout.ImagesBlobsDir+"/"+imageManifest.Digest.Encoded()),
			withTitle(imageConfig, layout.ImagesBlobsDir+"/"+imageConfig.Digest.Encoded()),
			withTitle(imageLayer, layout.ImagesBlobsDir+"/"+imageLayer.Digest.Encoded()),
		},
	}}
	_, err = boci.ToOCIRemote(packageRoot, ocispec.MediaTypeImageManifest, src.OrasRemote)
	require.NoError(t, err)

	p := RemotePusher{cfg: Config{PkgRootManifest: packageRoot, RemoteSrc: *src, RemoteDst: *dst}}
	packageDesc, err := p.Push()
	require.NoError(t, err)
	bundleConfig := content.NewDescriptorFromBytes(layout.ZarfLayerMediaTypeBlob, []byte("bundle config"))
	require.NoError(t, dst.Repo().Push(ctx, bundleConfig, bytes.NewReader([]byte("bundle config"))))
	bundleYAML := []byte("kind: UDSBundle\nmetadata:\n  name: test-bundle\n  version: v1\n  architecture: amd64\npackages:\n  - name: test\n    ref: test@sha256:" + packageDesc.Digest.Encoded() + "\n")
	bundleYAMLDesc := content.NewDescriptorFromBytes(layout.ZarfLayerMediaTypeBlob, bundleYAML)
	require.NoError(t, dst.Repo().Push(ctx, bundleYAMLDesc, bytes.NewReader(bundleYAML)))
	_, err = boci.ToOCIRemote(ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Config:    bundleConfig,
		Layers:    []ocispec.Descriptor{packageDesc, withTitle(bundleYAMLDesc, legacyconfig.BundleYAML)},
	}, ocispec.MediaTypeImageManifest, dst.OrasRemote)
	require.NoError(t, err)

	bundleRoot, err := dst.FetchRoot(ctx)
	require.NoError(t, err)
	require.Len(t, bundleRoot.Layers, 2)
	publishedYAML, err := dst.FetchLayer(ctx, bundleRoot.Locate(legacyconfig.BundleYAML))
	require.NoError(t, err)
	var publishedBundle legacytypes.UDSBundle
	require.NoError(t, goyaml.Unmarshal(publishedYAML, &publishedBundle))
	require.Len(t, publishedBundle.Packages, 1)
	require.Equal(t, "test@sha256:"+packageDesc.Digest.Encoded(), publishedBundle.Packages[0].Ref)
	publishedPackage, err := dst.FetchManifest(ctx, bundleRoot.Layers[0])
	require.NoError(t, err)
	definition, err := zoci.FetchZarfYAML(ctx, publishedPackage, dst)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{imageName, sharedImageName}, definition.Components[0].GetImages())
	imageLayers, err := zoci.LayersFromImages(ctx, publishedPackage, dst, map[string]bool{imageName: true, sharedImageName: true})
	require.NoError(t, err)
	for _, desc := range append(imageLayers, publishedPackage.Config) {
		_, err := content.FetchAll(ctx, dst, desc)
		require.NoError(t, err, "missing published image graph node %s", desc.Digest)
	}
}
