// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/pkg/logger"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	oraci "oras.land/oras-go/v2/content/oci"
)

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
		return nil, errEmpty("bundleDir")
	}
	if ociReference == "" {
		return nil, errEmpty("ociReference")
	}

	ociDir := filepath.Join(bundleDir, "oci")
	blobDir := filepath.Join(ociDir, "blobs", "sha256")

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
		return nil, fmt.Errorf("%s does not appear to be a UDS bundle: no bundle definition manifest found in index", bundleDir)
	}

	// Write the index bytes as a blob so the ORAS OCI store can fetch and serve it.
	// The oraci.Store serves content from blobs/<algo>/<hex>; adding the index there
	// allows store.Tag + oras.Copy to copy it (and recursively all referenced content)
	// to the remote registry.
	h := sha256.Sum256(idxBytes)
	idxHex := hex.EncodeToString(h[:])
	if err := os.WriteFile(filepath.Join(blobDir, idxHex), idxBytes, tmpFilePerm); err != nil {
		return nil, fmt.Errorf("staging index blob: %w", err)
	}

	store, err := oraci.New(ociDir)
	if err != nil {
		return nil, fmt.Errorf("opening OCI store: %w", err)
	}
	store.AutoSaveIndex = false

	// Build the index descriptor; pushToRemote will tag it before copying.
	idxDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, idxBytes)

	log := logger.Bind(opts.Streams, opts.Config.Global.LogLevel)
	log.Debug("copying bundle to registry", "ref", ociReference)
	result, err := pushToRemote(ctx, store, idxDesc, ociReference, &opts)
	if err != nil {
		return nil, err
	}
	log.Info("bundle pushed", "ref", ociReference)
	return result, nil
}

// PushPackage pushes a single Zarf package OCI layout from packageDir to a remote OCI registry.
// packageDir must contain an oci/ subdirectory with a valid OCI layout (index.json + blobs/).
func (p *defaultPusher) PushPackage(ctx context.Context, packageDir, ociReference string, opts PushOptions) (*PushResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if packageDir == "" {
		return nil, errEmpty("packageDir")
	}
	if ociReference == "" {
		return nil, errEmpty("ociReference")
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

// Push is a compatibility adapter that extracts the tarball and delegates to NewDefaultPusher().PushBundle.
// It preserves the current CLI tarball UX at the adapter layer.
func Push(ctx context.Context, bundleTarball, ociReference string, opts PushOptions) (*PushResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	s := logger.Bind(opts.Streams, opts.Config.Global.LogLevel)
	s.Info("pushing bundle", "tarball", bundleTarball, "ref", ociReference)

	tmp, err := os.MkdirTemp(opts.Config.Options.TmpDir, "uds-bundle-push-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() {
		if rerr := os.RemoveAll(tmp); rerr != nil {
			s.Warn("failed to remove temporary directory", "path", tmp, "error", rerr)
		}
	}()

	s.Debug("extracting bundle", "source", bundleTarball, "output", tmp)
	if err := extractTarZst(ctx, s, bundleTarball, tmp); err != nil {
		return nil, fmt.Errorf("extracting bundle: %w", err)
	}

	return NewDefaultPusher().PushBundle(ctx, tmp, ociReference, opts)
}
