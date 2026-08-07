// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	oraci "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
)

// defaultPusher provides the standard OCI push implementation.
type defaultPusher struct{}

var _ Pusher = (*defaultPusher)(nil)

// NewDefaultPusher returns the default Pusher implementation.
func NewDefaultPusher() Pusher { return &defaultPusher{} }

// PushBundle pushes an already-extracted bundle workspace to a remote OCI registry.
// bundleDir must contain an oci/ subdirectory with a valid OCI layout (index.json + blobs/).
func (p *defaultPusher) PushBundle(ctx context.Context, bundleDir, ociReference string, opts PushOptions) (*PushResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if bundleDir == "" {
		return nil, ErrEmpty("bundleDir")
	}
	if ociReference == "" {
		return nil, ErrEmpty("ociReference")
	}

	ociDir := filepath.Join(bundleDir, "oci")

	// Read the OCI image index from index.json.
	idxBytes, err := os.ReadFile(filepath.Join(ociDir, "index.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s does not appear to be a UDS bundle: no OCI layout found", bundleDir)
		}
		return nil, fmt.Errorf("reading index.json: %w", err)
	}

	var idx ociIndex
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		return nil, fmt.Errorf("parsing bundle index: %w", err)
	}
	if !isBundleIndex(idx) {
		return nil, fmt.Errorf("%s does not appear to be a UDS bundle: index does not declare artifactType %s", bundleDir, MediaTypeBundle)
	}
	arch := idx.Annotations[AnnotationBundleArchitecture]
	if arch == "" {
		return nil, fmt.Errorf("%s does not record its architecture: index is missing the %s annotation", bundleDir, AnnotationBundleArchitecture)
	}

	store, err := oraci.New(ociDir)
	if err != nil {
		return nil, fmt.Errorf("opening OCI store: %w", err)
	}
	store.AutoSaveIndex = false

	// The child (canonical single-arch bundle) descriptor: platform-tagged so it
	// can slot into the root index, artifact-typed so it is identifiable from
	// the root without a fetch (ADR-0015).
	childDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, idxBytes)
	childDesc.ArtifactType = MediaTypeBundle
	childDesc.Platform = &ocispec.Platform{Architecture: arch, OS: oci.MultiOS}

	// Stage the index bytes as a blob so the store can serve it: oras.CopyGraph
	// pushes the child (and, recursively, everything it references) from here.
	if err := store.Push(ctx, childDesc, bytes.NewReader(idxBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return nil, fmt.Errorf("staging index blob: %w", err)
	}

	log := logger.Bind(opts.Streams, opts.Config.Global.LogLevel)
	log.Debug("copying bundle to registry", "ref", ociReference, "arch", arch)
	result, err := pushBundleToRemote(ctx, store, childDesc, ociReference, &opts)
	if err != nil {
		return nil, err
	}
	log.Info("bundle pushed", "ref", ociReference, "arch", arch)
	return result, nil
}

// PushPackage pushes a single Zarf package OCI layout from packageDir to a remote OCI registry.
// packageDir must contain an oci/ subdirectory with a valid OCI layout (index.json + blobs/).
func (p *defaultPusher) PushPackage(ctx context.Context, packageDir, ociReference string, opts PushOptions) (*PushResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if packageDir == "" {
		return nil, ErrEmpty("packageDir")
	}
	if ociReference == "" {
		return nil, ErrEmpty("ociReference")
	}

	ociDir := filepath.Join(packageDir, "oci")
	store, err := oraci.New(ociDir)
	if err != nil {
		return nil, fmt.Errorf("opening OCI store: %w", err)
	}
	store.AutoSaveIndex = false

	root, err := packageRootDescriptor(ociDir)
	if err != nil {
		return nil, fmt.Errorf("reading OCI root descriptor: %w", err)
	}

	log := logger.Bind(opts.Streams, opts.Config.Global.LogLevel)
	log.Debug("pushing package", "ref", ociReference)
	result, err := pushToRemote(ctx, store, root, ociReference, &opts)
	if err != nil {
		return nil, err
	}
	log.Info("package pushed", "ref", ociReference)
	return result, nil
}
