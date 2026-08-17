// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	cosignbundle "github.com/sigstore/cosign/v3/pkg/cosign/bundle"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry"
)

// MediaTypeBundleSignature identifies standard Sigstore bundle evidence.
const MediaTypeBundleSignature = cosignbundle.BundleV03MediaType

// BundleSignatureFileName is the archive-root filename for bundle signature evidence.
const BundleSignatureFileName = "uds.bundle.sig"

// PublishBundleSignature publishes singleton Sigstore evidence for a child bundle index.
func PublishBundleSignature(ctx context.Context, target oras.Target, subject ocispec.Descriptor, data []byte, overwrite bool) error {
	store, ok := target.(content.ReadOnlyGraphStorage)
	if !ok {
		return fmt.Errorf("registry target does not support signature discovery")
	}
	refs, err := registry.Referrers(ctx, store, subject, MediaTypeBundleSignature)
	if err != nil {
		return fmt.Errorf("discovering existing bundle signature: %w", err)
	}
	if len(refs) == 1 {
		existing, err := signatureData(ctx, target, refs[0])
		if err != nil && !overwrite {
			return err
		}
		if err == nil && bytes.Equal(existing, data) {
			return nil
		}
	}
	var deleter content.Deleter
	if len(refs) != 0 {
		if !overwrite {
			return fmt.Errorf("bundle signature evidence already exists; set overwrite to replace it")
		}
		var ok bool
		deleter, ok = target.(content.Deleter)
		if !ok {
			return fmt.Errorf("registry target does not support replacing signature evidence")
		}
	}
	layer, err := PushBytes(ctx, target, MediaTypeBundleSignature, data, nil)
	if err != nil {
		return fmt.Errorf("pushing signature evidence: %w", err)
	}
	replacement, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, MediaTypeBundleSignature, oras.PackManifestOptions{
		Subject: &subject,
		Layers:  []ocispec.Descriptor{layer},
	})
	if err != nil {
		return fmt.Errorf("publishing signature artifact: %w", err)
	}
	for _, ref := range refs {
		if err := deleter.Delete(ctx, ref); err != nil && !errors.Is(err, errdef.ErrNotFound) {
			if rollbackErr := deleter.Delete(ctx, replacement); rollbackErr != nil && !errors.Is(rollbackErr, errdef.ErrNotFound) {
				return fmt.Errorf("removing replaced signature artifact: %w", errors.Join(err, fmt.Errorf("rolling back replacement signature artifact: %w", rollbackErr)))
			}
			return fmt.Errorf("removing replaced signature artifact: %w", err)
		}
	}
	return nil
}

func signatureData(ctx context.Context, source oras.Target, ref ocispec.Descriptor) ([]byte, error) {
	manifestBytes, err := FetchBytes(ctx, source, ref)
	if err != nil {
		return nil, fmt.Errorf("fetching signature artifact: %w", err)
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parsing signature artifact: %w", err)
	}
	if len(manifest.Layers) != 1 || manifest.Layers[0].MediaType != MediaTypeBundleSignature {
		return nil, fmt.Errorf("signature artifact has invalid layers")
	}
	data, err := FetchBytes(ctx, source, manifest.Layers[0])
	if err != nil {
		return nil, fmt.Errorf("fetching signature evidence: %w", err)
	}
	return data, nil
}

// FetchBundleSignature discovers and fetches Sigstore evidence for subject.
func FetchBundleSignature(ctx context.Context, source oras.Target, subject ocispec.Descriptor) ([]byte, error) {
	store, ok := source.(content.ReadOnlyGraphStorage)
	if !ok {
		return nil, fmt.Errorf("registry target does not support signature discovery")
	}
	refs, err := registry.Referrers(ctx, store, subject, MediaTypeBundleSignature)
	if err != nil {
		return nil, fmt.Errorf("discovering bundle signature: %w", err)
	}
	if len(refs) == 0 {
		return nil, ErrBundleSignatureNotFound
	}
	if len(refs) != 1 {
		return nil, fmt.Errorf("%w: expected exactly one bundle signature artifact, found %d", ErrBundleSignatureDuplicate, len(refs))
	}
	return signatureData(ctx, source, refs[0])
}
