// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	oras "oras.land/oras-go/v2"
	oraci "oras.land/oras-go/v2/content/oci"
)

type defaultPuller struct{}

var _ Puller = (*defaultPuller)(nil)

// NewDefaultPuller returns the default Puller implementation.
func NewDefaultPuller() Puller { return &defaultPuller{} }

// PullBundle pulls a UDS bundle from an OCI registry and writes it as a tar.zst
// archive to targetDir. It returns the path of the written archive.
//
// PullBundle uses oras.Copy to fetch the bundle index and all referenced blobs from
// the remote registry into a local OCI layout, then reconstructs index.json
// from the fetched root descriptor so the layout is identical to what Create
// produces. The resulting tarball can be pushed without modification.
func (p *defaultPuller) PullBundle(ctx context.Context, ociReference, targetDir string, opts PullOptions) (*PullResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if ociReference == "" {
		return nil, errEmpty("ociReference")
	}
	if targetDir == "" {
		return nil, errEmpty("targetDir")
	}

	tmp, err := os.MkdirTemp(opts.Config.Options.TmpDir, "uds-bundle-pull-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() {
		err = os.RemoveAll(tmp)
		if err != nil {
			slog.Warn("failed to remove temporary directory", "path", tmp, "error", err)
		}
	}()

	ociDir := filepath.Join(tmp, "oci")
	if err := os.MkdirAll(ociDir, tempDirPerm); err != nil {
		return nil, fmt.Errorf("creating OCI dir: %w", err)
	}

	// oraci.New writes oci-layout and initialises blobs/sha256/.
	store, err := oraci.New(ociDir)
	if err != nil {
		return nil, fmt.Errorf("creating OCI store: %w", err)
	}
	// We write index.json ourselves below; prevent ORAS from clobbering it.
	store.AutoSaveIndex = false

	var src oras.ReadOnlyTarget
	if opts.remoteRepo != nil {
		src = opts.remoteRepo
	} else {
		repo, err := newRemoteRepository(TrimScheme(ociReference), *opts.Config.Options)
		if err != nil {
			return nil, fmt.Errorf("creating remote repository: %w", err)
		}
		src = repo
	}

	ref, err := name.ParseReference(TrimScheme(ociReference))
	if err != nil {
		return nil, fmt.Errorf("parsing OCI reference: %w", err)
	}
	tag := ref.Identifier()

	copyOpts := oras.DefaultCopyOptions
	copyOpts.Concurrency = opts.Config.Options.Concurrency

	slog.Debug("copying bundle from registry", "ref", ociReference, "tag", tag)
	rootDesc, err := oras.Copy(ctx, src, tag, store, tag, copyOpts)
	if err != nil {
		return nil, fmt.Errorf("pulling bundle from %s: %w", ociReference, err)
	}

	// The root descriptor is the OCI image index that was pushed as the bundle.
	// Its bytes are stored verbatim in blobs/sha256/<hex>; fetch and write them
	// as index.json to restore the layout format produced by Create.
	rc, err := store.Fetch(ctx, rootDesc)
	if err != nil {
		return nil, fmt.Errorf("fetching bundle index: %w", err)
	}
	const maxIndexSize = 10 << 20 // 10 MiB
	idxBytes, err := io.ReadAll(io.LimitReader(rc, maxIndexSize+1))
	_ = rc.Close()
	if err != nil {
		return nil, fmt.Errorf("reading bundle index: %w", err)
	}
	if int64(len(idxBytes)) > maxIndexSize {
		return nil, fmt.Errorf("bundle index exceeds maximum allowed size of %d bytes", maxIndexSize)
	}

	if err := os.WriteFile(filepath.Join(ociDir, "index.json"), idxBytes, tmpFilePerm); err != nil {
		return nil, fmt.Errorf("writing index.json: %w", err)
	}

	var idx ociIndex
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		return nil, fmt.Errorf("parsing bundle index: %w", err)
	}

	// Validate this is a UDS bundle before doing anything else with the content.
	if !isBundleIndex(idx) {
		return nil, fmt.Errorf("%s does not appear to be a UDS bundle: no bundle definition manifest found in index", ociReference)
	}

	// oras.Copy stores the root index as a blob in addition to us writing it as
	// index.json. Remove it so the layout matches what Create produces (index
	// only in index.json, never as a blob).
	idxBlobPath := filepath.Join(ociDir, "blobs", "sha256", rootDesc.Digest.Hex())
	if err := os.Remove(idxBlobPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("removing duplicate index blob: %w", err)
	}

	outName, err := bundleNameFromDefinitionLayer(ctx, ociDir, idx, opts.Config.Options.Architecture)
	if err != nil {
		return nil, fmt.Errorf("reading bundle definition from %s: %w", ociReference, err)
	}

	outPath := filepath.Join(targetDir, outName)
	slog.Debug("writing bundle archive", "output", outPath)
	if err := writeTarZst(ctx, outPath, tmp); err != nil {
		return nil, err
	}

	slog.Info("bundle pulled", "output", outPath)
	return &PullResult{
		OCIReference: ociReference,
		OutputPath:   outPath,
	}, nil
}

// PullPackage is not yet implemented.
// TODO: implement single-package pull.
func (p *defaultPuller) PullPackage(_ context.Context, _, _ string, _ PullOptions) (*PullResult, error) {
	return nil, fmt.Errorf("PullPackage: %w", ErrNotImplemented)
}

// Pull is a compatibility adapter over NewDefaultPuller().PullBundle.
func Pull(ctx context.Context, ociReference, targetDir string, opts PullOptions) (*PullResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	slog.Info("pulling bundle", "ref", ociReference)
	return NewDefaultPuller().PullBundle(ctx, ociReference, targetDir, opts)
}

// bundleNameFromDefinitionLayer reads the bundle HCL from the bundle definition manifest
// in the OCI index and derives the output filename using bundleOutputName.
// It assumes isBundleIndex has already been called to confirm the index is valid.
func bundleNameFromDefinitionLayer(ctx context.Context, ociDir string, idx ociIndex, arch string) (string, error) {
	var cfgEntry *ociManifest
	for i := range idx.Manifests {
		if idx.Manifests[i].ArtifactType == MediaTypeBundleDefinition {
			cfgEntry = &idx.Manifests[i]
			break
		}
	}
	if cfgEntry == nil {
		return "", fmt.Errorf("bundle definition manifest not found in index")
	}

	cfgHex := strings.TrimPrefix(cfgEntry.Digest, "sha256:")
	cfgBytes, err := os.ReadFile(filepath.Join(ociDir, "blobs", "sha256", cfgHex))
	if err != nil {
		return "", fmt.Errorf("reading config manifest blob: %w", err)
	}

	var manifest ociImageManifest
	if err := json.Unmarshal(cfgBytes, &manifest); err != nil {
		return "", fmt.Errorf("parsing config manifest: %w", err)
	}

	var hclDigest string
	for _, l := range manifest.Layers {
		if l.MediaType == MediaTypeBundleHCL {
			hclDigest = l.Digest
			break
		}
	}
	if hclDigest == "" {
		return "", fmt.Errorf("bundle HCL layer not found in config manifest")
	}

	hclHex := strings.TrimPrefix(hclDigest, "sha256:")
	hclBytes, err := os.ReadFile(filepath.Join(ociDir, "blobs", "sha256", hclHex))
	if err != nil {
		return "", fmt.Errorf("reading HCL blob: %w", err)
	}

	b, err := NewHCLParser(arch).ParseBundleBytes(ctx, hclBytes)
	if err != nil {
		return "", fmt.Errorf("parsing bundle HCL: %w", err)
	}

	if arch == "" {
		arch = runtime.GOARCH
	}
	return bundleOutputName(b, arch), nil
}
