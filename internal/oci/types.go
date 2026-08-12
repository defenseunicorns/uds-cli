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
)

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

// GlobalOptions aliases the private global configuration options.
type GlobalOptions = bundleinternal.GlobalOptions

// PullOptions configures an OCI pull operation.
type PullOptions struct {
	Config    *Config
	Streams   iostreams.IOStreams
	PullHooks PullHooks
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
	CreateBundleArchive func(
		ctx context.Context,
		streams iostreams.IOStreams,
		ociDir, targetDir string,
		idx OciIndex,
		arch string,
	) (string, error)
}

// PushHooks provides extension points for OCI pushes.
type PushHooks struct {
	ToOrasTarget       func(ctx context.Context, ociReference string, opts *PushOptions) (oras.Target, error)
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
	// AnnotationPackageVerification records a successful package verification during bundle creation.
	AnnotationPackageVerification = "uds.dev/package-verification"
	// AnnotationPackageVerificationVerified is the persisted value for a successful verification.
	AnnotationPackageVerificationVerified = "verified"
	// AnnotationReconfiguredFrom records the source bundle's canonical child-index digest during reconfigure.
	AnnotationReconfiguredFrom = "org.defenseunicorns.uds.reconfigured-from"
)

// OciIndex is the top-level OCI image index written to index.json.
// For a UDS bundle child index, ArtifactType is MediaTypeBundle and
// Annotations carries AnnotationBundleArchitecture; a multi-arch root index
// has neither (it is a plain platform router, see ADR-0015).
type OciIndex struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType"`
	ArtifactType  string            `json:"artifactType,omitempty"`
	Manifests     []OciManifest     `json:"manifests"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}

// OciManifest is a descriptor entry inside an OCI image index.
type OciManifest struct {
	MediaType    string            `json:"mediaType"`
	ArtifactType string            `json:"artifactType,omitempty"`
	Digest       string            `json:"digest"`
	Size         int64             `json:"size"`
	Platform     *specv1.Platform  `json:"platform,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
}

// ociLayout is the content of the oci-layout marker file.
type ociLayout struct {
	ImageLayoutVersion string `json:"imageLayoutVersion"`
}

// OciDescriptor is a generic OCI content descriptor used inside image manifests.
type OciDescriptor struct {
	MediaType   string            `json:"mediaType,omitempty"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	URLs        []string          `json:"urls,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// OciImageManifest is the image manifest JSON blob referenced by an ociManifest entry.
type OciImageManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType,omitempty"`
	ArtifactType  string            `json:"artifactType,omitempty"`
	Config        OciDescriptor     `json:"config"`
	Layers        []OciDescriptor   `json:"layers"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}

// ociIndex preserves the package-private name for the exported OCI index model.
type ociIndex = OciIndex

// ociManifest preserves the package-private name for the exported OCI manifest model.
type ociManifest = OciManifest

// ociImageManifest preserves the package-private name for the exported OCI image manifest model.
type ociImageManifest = OciImageManifest

// ociDescriptor preserves the package-private name for the exported OCI descriptor model.
type ociDescriptor = OciDescriptor
