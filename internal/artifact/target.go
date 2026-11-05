// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/defenseunicorns/uds-cli/internal/filesystem"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
)

// MaterializeTargetDefinition writes bundle files from a read-only ORAS target.
// It does not fetch packages; the caller cleans up dstDir.
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
	bundlePath, err := materializeBundleSrcFilesWithWriter(ctx, streams, read.Manifest, dstDir, func(ctx context.Context, layer ocispec.Descriptor, root *os.Root, path string) error {
		title := layer.Annotations[ocispec.AnnotationTitle]
		if layer.Digest == read.HCLLayer.Digest && layer.Size == read.HCLLayer.Size {
			if err := root.WriteFile(path, read.HCLBytes, filesystem.PrivateFileMode); err != nil {
				return WritingLayerError{Title: title, Err: err}
			}
			return nil
		}
		file, err := root.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, filesystem.PrivateFileMode)
		if err != nil {
			return WritingLayerError{Title: title, Err: err}
		}
		copyErr := udsoci.FetchVerifiedTo(ctx, target, layer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return ReadingLayerBlobError{Title: title, Digest: layer.Digest, Err: errors.Join(copyErr, closeErr)}
		}
		if closeErr != nil {
			return WritingLayerError{Title: title, Err: closeErr}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &ExtractedBundle{Dir: dstDir, BundleDefPath: bundlePath, ArtifactDigest: child.Digest.String()}, read.Inspection.Bundle, nil
}
