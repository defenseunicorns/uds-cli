// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"io/fs"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	orasoci "oras.land/oras-go/v2/content/oci"
)

// Store is a bundle OCI layout backed by ORAS content storage.
//
// ORAS owns content-addressed writes, digest verification, and atomic storage.
// Store adds only filesystem operations ORAS intentionally does not expose:
// locating blobs for package staging and enumerating blobs for garbage collection.
type Store struct {
	*orasoci.Store
	root string
}

// Target is an OCI content target used by ORAS-backed registry and test stores.
type Target = oras.Target

// Fetcher fetches descriptor-addressed OCI content.
type Fetcher = content.Fetcher

const (
	// Permissions for directories and files created by OCI operations.
	tempDirPerm fs.FileMode = 0o700
	tmpFilePerm fs.FileMode = 0o600
)

// Puller is the interface for pulling bundle artifacts from an OCI registry.
// OCIReference and targetDir are method-level parameters; config and extensibility
// belong in PullOptions. This matches the Technical Design Doc surface.
type Puller interface {
	// PullBundle pulls a bundle from the given OCI reference and writes it to targetDir.
	PullBundle(ctx context.Context, ociReference, targetDir string, opts PullOptions) (*PullResult, error)
	// PullPackage pulls a single Zarf package from the given OCI reference to targetDir.
	PullPackage(ctx context.Context, ociReference, targetDir string, opts PullOptions) (*PullResult, error)
}

// Pusher is the interface for pushing bundle artifacts to an OCI registry.
// bundleDir/packageDir and OCIReference are method-level parameters; config and
// extensibility belong in PushOptions.
type Pusher interface {
	// PushBundle pushes the OCI layout in bundleDir to the given OCI reference.
	// bundleDir must contain an oci/ subdirectory with a valid OCI layout (index.json + blobs/).
	PushBundle(ctx context.Context, bundleDir, ociReference string, opts PushOptions) (*PushResult, error)
	// PushPackage pushes a single Zarf package from packageDir to the given OCI reference.
	PushPackage(ctx context.Context, packageDir, ociReference string, opts PushOptions) (*PushResult, error)
}

// Config aliases the private bundle configuration used by OCI operations.
type Config = bundleinternal.UDSBundleConfig

// UDSBundleConfig aliases the private bundle configuration for migrated callers.
type UDSBundleConfig = bundleinternal.UDSBundleConfig

// ConfigOptions aliases the private OCI-relevant configuration options.
type ConfigOptions = bundleinternal.ConfigOptions

// PullOptions configures an OCI pull operation.
type PullOptions struct {
	Config                    *Config
	Streams                   iostreams.IOStreams
	SkipSignatureVerification bool
	PullHooks                 PullHooks
}

// Validate validates pull options.
func (o PullOptions) Validate() error {
	return bundleinternal.ValidateConfig(o.Config)
}

// PushOptions configures an OCI push operation.
type PushOptions struct {
	Config    *Config
	Streams   iostreams.IOStreams
	PushHooks PushHooks
}

// Validate validates push options.
func (o PushOptions) Validate() error {
	return bundleinternal.ValidateConfig(o.Config)
}

// PullResult describes a completed OCI pull.
type PullResult struct {
	OCIReference string `json:"ociReference" yaml:"ociReference" text:"OCI Reference"`
	OutputPath   string `json:"outputPath" yaml:"outputPath" text:"Output Path"`
}

// PushResult describes a completed OCI push.
type PushResult struct {
	OCIReference string `json:"ociReference" yaml:"ociReference" text:"OCI Reference"`
}

// PullHooks provides extension points for OCI pulls.
type PullHooks struct {
	ToOrasTarget        func(ctx context.Context, ociReference string, opts *PullOptions) (oras.Target, error)
	ModifyOrasSettings  func(ctx context.Context, copyOptions *oras.CopyOptions) error
	VerifyBundle        func(ctx context.Context, index, evidence []byte) error
	CreateBundleArchive func(
		ctx context.Context,
		streams iostreams.IOStreams,
		ociDir, targetDir string,
		idx specv1.Index,
		arch string,
	) (string, error)
}

// PushHooks provides extension points for OCI pushes.
type PushHooks struct {
	ToOrasTarget func(ctx context.Context, ociReference string, opts *PushOptions) (oras.Target, error)
	// ModifyOrasSettings is not called when a bundle push is already fully published and no copy is required.
	ModifyOrasSettings func(ctx context.Context, copyOptions *oras.CopyOptions) error
}

const (
	// Media types for UDS bundle OCI artifacts.
	MediaTypeBundleDefinition = "application/vnd.defenseunicorns.uds.bundle.definition.v1"
	MediaTypeBundleHCL        = "application/vnd.defenseunicorns.uds.bundle.hcl.v1"
	MediaTypeBundleValuesYAML = "application/vnd.defenseunicorns.uds.bundle.values.v1+yaml"

	// MediaTypeBundle is the artifactType of the canonical single-arch bundle
	// index (the child index a published tag's root index points at, and the
	// index.json inside a bundle .tar.zst). See ADR-0015.
	MediaTypeBundle = "application/vnd.defenseunicorns.uds.bundle.v1"

	// MediaTypeZarfLayer is the media type for Zarf package file layers.
	MediaTypeZarfLayer = "application/vnd.defenseunicorns.zarf.layer.v1"
)

// AnnotationBundleArchitecture is the child bundle index annotation recording
// the single architecture the bundle was built for. Member package entries
// carry no platform field (ADR-0015), so this keeps the artifact
// self-describing and lets push populate the root index's platform entry.
const AnnotationBundleArchitecture = "uds.dev/architecture"

const (
	// AnnotationPackageName records the bundle package name that identifies a package descriptor.
	AnnotationPackageName = "uds.dev/package.name"
	// AnnotationPackageSource records the bundle package source for provenance.
	AnnotationPackageSource = "uds.dev/package.source"
	// AnnotationPackageVerification records a successful package verification during bundle creation.
	AnnotationPackageVerification = "uds.dev/package-verification"
	// AnnotationPackageVerificationVerified is the persisted value for a successful verification.
	AnnotationPackageVerificationVerified = "verified"
	// AnnotationReconfiguredFrom records the source bundle's canonical child-index digest during reconfigure.
	AnnotationReconfiguredFrom = "org.defenseunicorns.uds.reconfigured-from"
)
