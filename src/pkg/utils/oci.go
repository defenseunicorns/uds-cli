// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package utils provides utility fns for UDS-CLI
package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"
)

// FetchLayerAndStore fetches a remote layer and copies it to a local store
func FetchLayerAndStore(layerDesc ocispec.Descriptor, remoteRepo *oci.OrasRemote, localStore *ocistore.Store) error {
	layerBytes, err := remoteRepo.FetchLayer(layerDesc)
	if err != nil {
		return err
	}
	rootPkgDescBytes := content.NewDescriptorFromBytes(oci.ZarfLayerMediaTypeBlob, layerBytes)
	err = localStore.Push(context.TODO(), rootPkgDescBytes, bytes.NewReader(layerBytes))
	return err
}

// ToOCIStore takes an arbitrary type, typically a struct, marshals it into JSON and store it in a local OCI store
func ToOCIStore(t any, mediaType string, store *ocistore.Store) (ocispec.Descriptor, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	desc := content.NewDescriptorFromBytes(mediaType, b)
	if err := store.Push(context.TODO(), desc, bytes.NewReader(b)); err != nil {
		return ocispec.Descriptor{}, err
	}
	return desc, nil
}

// ToOCIRemote takes an arbitrary type, typically a struct, marshals it into JSON and store it in a remote OCI store
func ToOCIRemote(t any, mediaType string, remote *oci.OrasRemote) (ocispec.Descriptor, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	var layerDesc ocispec.Descriptor
	// if image manifest media type, push to Manifests(), otherwise normal pushLayer()
	if mediaType == ocispec.MediaTypeImageManifest {
		layerDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, b)
		if err := remote.Repo().Manifests().PushReference(context.TODO(), layerDesc, bytes.NewReader(b), remote.Repo().Reference.String()); err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("failed to push manifest: %w", err)
		}
	} else {
		layerDesc, err = remote.PushLayer(b, mediaType)
		if err != nil {
			return ocispec.Descriptor{}, err
		}
	}

	message.Successf("Published %s [%s]", remote.Repo().Reference.String(), layerDesc.MediaType)
	return layerDesc, nil
}

// CreateCopyOpts creates the ORAS CopyOpts struct to use when copying OCI artifacts
func CreateCopyOpts(layersToPull []ocispec.Descriptor, concurrency int) oras.CopyOptions {
	var copyOpts oras.CopyOptions
	copyOpts.Concurrency = concurrency
	estimatedBytes := int64(0)
	var shas []string
	for _, layer := range layersToPull {
		if len(layer.Digest.String()) > 0 {
			estimatedBytes += layer.Size
			shas = append(shas, layer.Digest.Encoded())
		}
	}
	copyOpts.FindSuccessors = func(ctx context.Context, fetcher content.Fetcher, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		var nodes []ocispec.Descriptor
		if desc.MediaType == oci.ZarfLayerMediaTypeBlob && desc.Annotations == nil {
			layerBytes, err := content.FetchAll(ctx, fetcher, desc)
			if err != nil {
				return nil, err
			}
			var manifest oci.ZarfOCIManifest
			if err := json.Unmarshal(layerBytes, &manifest); err != nil {
				return nil, err
			}
			if manifest.Subject != nil {
				nodes = append(nodes, *manifest.Subject)
			}
			nodes = append(nodes, manifest.Config)
			nodes = append(nodes, manifest.Layers...)
		} else {
			successors, err := content.Successors(ctx, fetcher, desc)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, successors...)
		}
		var ret []ocispec.Descriptor
		for _, node := range nodes {
			if node.Size != 0 && slices.Contains(shas, node.Digest.Encoded()) {
				ret = append(ret, node)
			}
		}
		return ret, nil
	}
	return copyOpts
}
