// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/filesystem"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	oraci "oras.land/oras-go/v2/content/oci"
)

// Puller pulls bundle artifacts from an OCI registry.
type Puller interface {
	// PullBundle pulls a bundle from the given OCI reference and writes it to targetDir.
	PullBundle(ctx context.Context, ociReference, targetDir string, opts PullOptions) (*PullResult, error)
	// PullPackage pulls a single Zarf package from the given OCI reference to targetDir.
	PullPackage(ctx context.Context, ociReference, targetDir string, opts PullOptions) (*PullResult, error)
}

// PullOptions configures an OCI pull operation.
type PullOptions struct {
	Config                    *bundleinternal.UDSBundleConfig
	Streams                   iostreams.IOStreams
	SkipSignatureVerification bool
	PullHooks                 PullHooks
}

// Validate validates pull options.
func (o PullOptions) Validate() error { return bundleinternal.ValidateConfig(o.Config) }

// PullResult describes a completed OCI pull.
type PullResult struct {
	OCIReference string `json:"ociReference" yaml:"ociReference" text:"OCI Reference"`
	OutputPath   string `json:"outputPath" yaml:"outputPath" text:"Output Path"`
}

// PullHooks provides extension points for OCI pulls.
type PullHooks struct {
	ToOrasTarget        func(ctx context.Context, ociReference string, opts *PullOptions) (oras.Target, error)
	ModifyOrasSettings  func(ctx context.Context, copyOptions *oras.CopyOptions) error
	VerifyBundle        func(ctx context.Context, index, evidence []byte) error
	CreateBundleArchive func(ctx context.Context, streams iostreams.IOStreams, ociDir, targetDir string, idx ocispec.Index, arch string) (string, error)
}

// defaultPuller provides the standard OCI pull implementation.
type defaultPuller struct{}

var _ Puller = (*defaultPuller)(nil)

// NewDefaultPuller returns the default Puller implementation.
func NewDefaultPuller() Puller { return &defaultPuller{} }

// PullBundle pulls a UDS bundle from an OCI registry and writes it as a tar.zst
// archive to targetDir. It returns the path of the written archive.
//
// PullBundle uses ORAS graph copy to fetch the selected bundle index and all
// referenced blobs from the remote registry into a local OCI layout, then reconstructs index.json
// from the fetched root descriptor so the layout is identical to what Create
// produces. The resulting tarball can be pushed without modification.
func (p *defaultPuller) PullBundle(ctx context.Context, ociReference, targetDir string, opts PullOptions) (*PullResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if ociReference == "" {
		return nil, ErrEmpty("ociReference")
	}
	if targetDir == "" {
		return nil, ErrEmpty("targetDir")
	}

	log := logger.Bind(opts.Streams, opts.Config.Options.LogLevel)
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
	if err := os.MkdirAll(ociDir, filesystem.PrivateDirectoryMode); err != nil {
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
	reference, err := ReferenceIdentifier(ociReference)
	if err != nil {
		return nil, err
	}

	// Resolve to the canonical single-arch bundle (child) index: a tag resolves
	// to the root index and is platform-selected for the requested architecture;
	// a digest-pinned reference addresses a child directly (ADR-0015).
	childDesc, idxBytes, err := ResolveBundleChild(ctx, src, reference, opts.Config.Options.Architecture)
	if err != nil {
		return nil, fmt.Errorf("resolving bundle from %s: %w", ociReference, err)
	}
	var signature []byte
	if opts.PullHooks.VerifyBundle != nil {
		signature, err = FetchBundleSignature(ctx, src, childDesc)
		if err != nil {
			return nil, fmt.Errorf("fetching bundle signature evidence: %w", err)
		}
		if err := opts.PullHooks.VerifyBundle(ctx, idxBytes, signature); err != nil {
			return nil, fmt.Errorf("verifying bundle signature: %w", err)
		}
	}

	// Copy only the selected architecture's graph — never sibling architectures.
	copyOpts, err := pullCopyOptions(ctx, &opts)
	if err != nil {
		return nil, fmt.Errorf("configuring pull: %w", err)
	}
	log.Info("pulling bundle content", "ref", ociReference)
	log.Debug("copying bundle from registry", "ref", ociReference, "digest", childDesc.Digest.String())
	if err := copyGraph(ctx, src, store, childDesc, copyOpts.CopyGraphOptions); err != nil {
		return nil, fmt.Errorf("pulling bundle from %s: %w", ociReference, err)
	}
	if signature == nil {
		signature, err = FetchBundleSignature(ctx, src, childDesc)
	}
	if err == nil {
		if err := os.WriteFile(filepath.Join(tmp, BundleSignatureFileName), signature, filesystem.PrivateFileMode); err != nil {
			return nil, fmt.Errorf("writing bundle signature evidence: %w", err)
		}
	} else if errors.Is(err, ErrBundleSignatureNotFound) {
		log.Debug("bundle signature evidence unavailable", "error", err)
	} else if opts.SkipSignatureVerification {
		log.Warn("unable to preserve bundle signature evidence; continuing because signature verification was skipped; later verification may fail", "error", err)
	} else {
		return nil, fmt.Errorf("fetching bundle signature evidence: %w", err)
	}

	// Write the child index bytes verbatim as index.json to restore the layout
	// format produced by Create (round-trips byte-identically).
	if err := os.WriteFile(filepath.Join(ociDir, "index.json"), idxBytes, filesystem.PrivateFileMode); err != nil {
		return nil, fmt.Errorf("writing index.json: %w", err)
	}

	var idx ocispec.Index
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		return nil, fmt.Errorf("parsing bundle index: %w", err)
	}

	// Graph copy stores the child index as a blob in addition to us writing it as
	// index.json. Remove it so the layout matches what Create produces
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
	if opts.PullHooks.CreateBundleArchive == nil {
		return nil, fmt.Errorf("creating bundle archive: archive hook is required")
	}
	log.Info("writing bundle archive", "output_dir", targetDir)
	outPath, err := opts.PullHooks.CreateBundleArchive(ctx, log, ociDir, targetDir, idx, outArch)
	if err != nil {
		return nil, fmt.Errorf("creating bundle archive from %s: %w", ociReference, err)
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
		return nil, ErrEmpty("ociReference")
	}
	if targetDir == "" {
		return nil, ErrEmpty("targetDir")
	}

	log := logger.Bind(opts.Streams, opts.Config.Options.LogLevel)

	ociDir := filepath.Join(targetDir, "oci")
	if err := os.MkdirAll(ociDir, filesystem.PrivateDirectoryMode); err != nil {
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
