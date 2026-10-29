// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"context"
	"fmt"
	"runtime"

	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
)

// MaterializeTargetDefinition prepares bundle-owned files from an ORAS target.
// The caller owns dstDir and its cleanup. Package payloads are never fetched.
func MaterializeTargetDefinition(ctx context.Context, streams iostreams.IOStreams, target oras.ReadOnlyTarget, reference, architecture, dstDir string) (*ExtractedBundle, *spec.UDSBundle, error) {
	if architecture == "" {
		architecture = runtime.GOARCH
	}
	child, indexBytes, err := udsoci.ResolveBundleChild(ctx, target, reference, architecture)
	if err != nil {
		return nil, nil, err
	}
	metadata := &MetadataSource{IndexBytes: indexBytes, ArtifactDigest: child.Digest.String(), Fetcher: target}
	read, err := readBundleDefinition(ctx, metadata, streams)
	if err != nil {
		return nil, nil, err
	}
	if found := read.Index.Annotations[udsoci.AnnotationBundleArchitecture]; found != architecture {
		return nil, nil, fmt.Errorf("bundle architecture %q does not match requested architecture %q: %w", found, architecture, udsoci.ErrArchitectureUnavailable)
	}
	bundlePath, err := materializeBundleSrcFilesWithReader(ctx, streams, read.Manifest, dstDir, func(ctx context.Context, layer ocispec.Descriptor) ([]byte, error) {
		if layer.Digest == read.HCLLayer.Digest && layer.Size == read.HCLLayer.Size {
			return read.HCLBytes, nil
		}
		return udsoci.FetchBytes(ctx, target, layer)
	})
	if err != nil {
		return nil, nil, err
	}
	return &ExtractedBundle{Dir: dstDir, BundleDefPath: bundlePath, ArtifactDigest: child.Digest.String()}, read.Inspection.Bundle, nil
}
