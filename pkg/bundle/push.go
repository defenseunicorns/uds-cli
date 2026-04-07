// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/name"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	oraci "oras.land/oras-go/v2/content/oci"
)

// Push pushes a UDS bundle tarball to a remote OCI registry.
// It extracts the bundle, opens the embedded OCI layout, and copies all
// content (blobs, manifests, and the top-level image index) to the registry
// reference given in opts.OCIReference.
func Push(ctx context.Context, opts PushOptions) (*PushResult, error) {
	slog.Info("pushing bundle", "tarball", opts.BundleTarball, "ref", opts.OCIReference)

	tmp, err := os.MkdirTemp(opts.TmpDir, "uds-bundle-push-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() {
		err = os.RemoveAll(tmp)
		if err != nil {
			slog.Warn("failed to remove temporary directory", "path", tmp, "error", err)
		}
	}()

	slog.Debug("extracting bundle", "source", opts.BundleTarball, "output", tmp)
	if err := extractTarZst(ctx, opts.BundleTarball, tmp); err != nil {
		return nil, fmt.Errorf("extracting bundle: %w", err)
	}

	ociDir := filepath.Join(tmp, "oci")
	blobDir := filepath.Join(ociDir, "blobs", "sha256")

	// Read the OCI image index from index.json.
	idxBytes, err := os.ReadFile(filepath.Join(ociDir, "index.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s does not appear to be a UDS bundle: no OCI layout found", opts.BundleTarball)
		}
		return nil, fmt.Errorf("reading index.json: %w", err)
	}

	var idx ociIndex
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		return nil, fmt.Errorf("parsing bundle index: %w", err)
	}
	if !isBundleIndex(idx) {
		return nil, fmt.Errorf("%s does not appear to be a UDS bundle: no bundle definition manifest found in index", opts.BundleTarball)
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

	// Build the index descriptor and tag it so oras.Copy can resolve it.
	idxDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, idxBytes)
	if err := store.Tag(ctx, idxDesc, "bundle"); err != nil {
		return nil, fmt.Errorf("tagging index: %w", err)
	}

	var dst oras.Target
	if opts.remoteRepo != nil {
		dst = opts.remoteRepo
	} else {
		repo, err := newRemoteRepository(TrimScheme(opts.OCIReference), opts.RegistryOptions)
		if err != nil {
			return nil, fmt.Errorf("creating remote repository: %w", err)
		}
		dst = repo
	}

	// Parse the tag/digest from the reference using go-containerregistry; default to "latest".
	ref, err := name.ParseReference(TrimScheme(opts.OCIReference))
	if err != nil {
		return nil, fmt.Errorf("parsing OCI reference: %w", err)
	}
	dstTag := ref.Identifier()

	slog.Debug("copying bundle to registry", "ref", opts.OCIReference, "tag", dstTag)
	if _, err := oras.Copy(ctx, store, "bundle", dst, dstTag, oras.DefaultCopyOptions); err != nil {
		return nil, fmt.Errorf("pushing bundle to %s: %w", opts.OCIReference, err)
	}

	slog.Info("bundle pushed", "ref", opts.OCIReference)
	return &PushResult{
		OCIReference: opts.OCIReference,
	}, nil
}

