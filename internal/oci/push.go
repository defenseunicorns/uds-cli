// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
)

// Pusher pushes bundle artifacts to an OCI registry.
type Pusher interface {
	// PushBundle pushes the OCI layout in bundleDir to the given OCI reference.
	// bundleDir must contain an oci/ subdirectory with a valid OCI layout (index.json + blobs/).
	PushBundle(ctx context.Context, bundleDir, ociReference string, opts PushOptions) (*PushResult, error)
	// PushPackage pushes a single Zarf package from packageDir to the given OCI reference.
	PushPackage(ctx context.Context, packageDir, ociReference string, opts PushOptions) (*PushResult, error)
}

// PushOptions configures an OCI push operation.
type PushOptions struct {
	Config    *bundleinternal.UDSBundleConfig
	Streams   iostreams.IOStreams
	PushHooks PushHooks
}

// Validate validates push options.
func (o PushOptions) Validate() error { return bundleinternal.ValidateConfig(o.Config) }

// PushResult describes a completed OCI push.
type PushResult struct {
	OCIReference string `json:"ociReference" yaml:"ociReference" text:"OCI Reference"`
}

// PushHooks provides extension points for OCI pushes.
type PushHooks struct {
	ToOrasTarget func(ctx context.Context, ociReference string, opts *PushOptions) (oras.Target, error)
	// ModifyOrasSettings is not called when a bundle push is already fully published and no copy is required.
	ModifyOrasSettings func(ctx context.Context, copyOptions *oras.CopyOptions) error
}

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
		return nil, EmptyParameterError{Name: "bundleDir"}
	}
	if ociReference == "" {
		return nil, EmptyParameterError{Name: "ociReference"}
	}

	ociDir := filepath.Join(bundleDir, "oci")
	indexPath := filepath.Join(ociDir, "index.json")

	// Read the OCI image index from index.json.
	idxBytes, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s does not appear to be a UDS bundle: no OCI layout found: %w: %w", bundleDir, ErrInvalidBundle, err)
		}
		return nil, fmt.Errorf("%w %q: %w", ErrReadIndex, indexPath, err)
	}

	var idx ocispec.Index
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		return nil, fmt.Errorf("parsing bundle index: %w: %w", ErrParseIndex, err)
	}
	if !IsBundleIndex(idx) {
		return nil, fmt.Errorf("%s does not appear to be a UDS bundle: index does not declare artifactType %s: %w", bundleDir, MediaTypeBundle, ErrInvalidBundle)
	}
	arch := idx.Annotations[AnnotationBundleArchitecture]
	if arch == "" {
		return nil, fmt.Errorf("%s does not record its architecture: index is missing the %s annotation: %w", bundleDir, AnnotationBundleArchitecture, ErrMissingArchitecture)
	}

	store, err := OpenStore(ociDir)
	if err != nil {
		return nil, err
	}

	// The child (canonical single-arch bundle) descriptor: platform-tagged so it
	// can slot into the root index, artifact-typed so it is identifiable from
	// the root without a fetch (ADR-0015).
	childDesc := BundleChildDescriptor(content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, idxBytes), arch)
	signature, readErr := os.ReadFile(filepath.Join(bundleDir, BundleSignatureFileName))
	hasSignature := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return nil, fmt.Errorf("reading bundle signature evidence: %w", readErr)
	}

	// Stage the index bytes as a blob so graph copy can push the child index and
	// everything it references from this local store.
	if err := PushDescriptorBytes(ctx, store, childDesc, idxBytes); err != nil {
		return nil, fmt.Errorf("staging index blob: %w: %w", ErrPushContent, err)
	}

	log := logger.Bind(opts.Streams, opts.Config.Options.LogLevel)
	log.Info("pushing bundle content", "ref", ociReference, "arch", arch)
	log.Debug("copying bundle to registry", "ref", ociReference, "arch", arch)
	result, err := pushBundleToRemote(ctx, store.Store, childDesc, ociReference, &opts, signature, hasSignature)
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
		return nil, EmptyParameterError{Name: "packageDir"}
	}
	if ociReference == "" {
		return nil, EmptyParameterError{Name: "ociReference"}
	}

	ociDir := filepath.Join(packageDir, "oci")
	store, err := OpenStore(ociDir)
	if err != nil {
		return nil, err
	}

	root, err := packageRootDescriptor(ociDir)
	if err != nil {
		return nil, fmt.Errorf("%w from %q: %w", ErrReadRootDescriptor, ociDir, err)
	}

	log := logger.Bind(opts.Streams, opts.Config.Options.LogLevel)
	log.Debug("pushing package", "ref", ociReference)
	result, err := pushToRemote(ctx, store.Store, root, ociReference, &opts)
	if err != nil {
		return nil, err
	}
	log.Info("package pushed", "ref", ociReference)
	return result, nil
}
