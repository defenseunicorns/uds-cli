// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/logger"
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

	log := logger.Bind(opts.Streams, opts.Config.Global.LogLevel)
	tmp, err := os.MkdirTemp(opts.Config.Options.TmpDir, "uds-bundle-pull-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() {
		err = os.RemoveAll(tmp)
		if err != nil {
			log.Warn("failed to remove temporary directory", "path", tmp, "error", err)
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

	src, err := resolvePullTarget(ctx, ociReference, &opts)
	if err != nil {
		return nil, fmt.Errorf("resolving pull source %s: %w", ociReference, err)
	}
	reference, err := refIdentifier(ociReference)
	if err != nil {
		return nil, err
	}

	// Resolve to the canonical single-arch bundle (child) index: a tag resolves
	// to the root index and is platform-selected for the requested architecture;
	// a digest-pinned reference addresses a child directly (ADR-0015).
	childDesc, idxBytes, err := resolveBundleChild(ctx, src, reference, opts.Config.Options.Architecture)
	if err != nil {
		return nil, fmt.Errorf("resolving bundle from %s: %w", ociReference, err)
	}

	// Copy only the selected architecture's graph — never sibling architectures.
	copyOpts, err := pullCopyOptions(ctx, &opts)
	if err != nil {
		return nil, fmt.Errorf("configuring pull: %w", err)
	}
	log.Debug("copying bundle from registry", "ref", ociReference, "digest", childDesc.Digest.String())
	if err := oras.CopyGraph(ctx, src, store, childDesc, copyOpts.CopyGraphOptions); err != nil {
		return nil, fmt.Errorf("pulling bundle from %s: %w", ociReference, err)
	}

	// Write the child index bytes verbatim as index.json to restore the layout
	// format produced by Create (round-trips byte-identically).
	if err := os.WriteFile(filepath.Join(ociDir, "index.json"), idxBytes, tmpFilePerm); err != nil {
		return nil, fmt.Errorf("writing index.json: %w", err)
	}

	var idx ociIndex
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		return nil, fmt.Errorf("parsing bundle index: %w", err)
	}

	// oras.CopyGraph stores the child index as a blob in addition to us writing
	// it as index.json. Remove it so the layout matches what Create produces
	// (index only in index.json, never as a blob).
	idxBlobPath := filepath.Join(ociDir, "blobs", "sha256", childDesc.Digest.Hex())
	if err := os.Remove(idxBlobPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("removing duplicate index blob: %w", err)
	}

	// Name the archive after the child's own recorded architecture — for a
	// digest-pinned pull it may differ from the requested/host architecture.
	outArch := idx.Annotations[AnnotationBundleArchitecture]
	if outArch == "" {
		return nil, fmt.Errorf("encountered corrupted bundle definition %s: missing architecture annotation", ociReference)
	}
	outName, err := bundleNameFromDefinitionLayer(ctx, log, ociDir, idx, outArch)
	if err != nil {
		return nil, fmt.Errorf("reading bundle definition from %s: %w", ociReference, err)
	}

	outPath := filepath.Join(targetDir, outName)
	log.Debug("writing bundle archive", "output", outPath)
	if err := writeTarZst(ctx, log, outPath, tmp); err != nil {
		return nil, err
	}

	log.Info("bundle pulled", "output", outPath)
	return &PullResult{
		OCIReference: ociReference,
		OutputPath:   outPath,
	}, nil
}

// PullPackage pulls a single Zarf package from an OCI registry into an OCI layout
// directory at <targetDir>/oci. The layout is left on disk for cross-mount use.
func (p *defaultPuller) PullPackage(ctx context.Context, ociReference, targetDir string, opts PullOptions) (*PullResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if ociReference == "" {
		return nil, errEmpty("ociReference")
	}
	if targetDir == "" {
		return nil, errEmpty("targetDir")
	}

	log := logger.Bind(opts.Streams, opts.Config.Global.LogLevel)

	ociDir := filepath.Join(targetDir, "oci")
	if err := os.MkdirAll(ociDir, tempDirPerm); err != nil {
		return nil, fmt.Errorf("creating OCI dir: %w", err)
	}
	store, err := oraci.New(ociDir)
	if err != nil {
		return nil, fmt.Errorf("creating OCI store: %w", err)
	}

	if _, err := pullToStore(ctx, ociReference, store, &opts); err != nil {
		return nil, fmt.Errorf("pulling package from %s: %w", ociReference, err)
	}

	log.Info("package pulled", "output", ociDir)
	return &PullResult{OCIReference: ociReference, OutputPath: ociDir}, nil
}

// Pull is a compatibility adapter over NewDefaultPuller().PullBundle.
func Pull(ctx context.Context, ociReference, targetDir string, opts PullOptions) (*PullResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	s := logger.Bind(opts.Streams, opts.Config.Global.LogLevel)
	s.Info("pulling bundle", "ref", ociReference)
	return NewDefaultPuller().PullBundle(ctx, ociReference, targetDir, opts)
}

// bundleNameFromDefinitionLayer reads the bundle HCL from the bundle definition manifest
// in the OCI index and derives the output filename using bundleOutputName.
// It assumes isBundleIndex has already been called to confirm the index is valid.
func bundleNameFromDefinitionLayer(ctx context.Context, streams iostreams.IOStreams, ociDir string, idx ociIndex, arch string) (string, error) {
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

	b, err := NewHCLParser(arch, streams).ParseBundleBytes(ctx, hclBytes)
	if err != nil {
		return "", fmt.Errorf("parsing bundle HCL: %w", err)
	}

	if arch == "" {
		arch = runtime.GOARCH
	}
	return bundleOutputName(b, arch), nil
}
