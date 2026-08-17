// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	oras "oras.land/oras-go/v2"
)

const (
	// MediaTypeBundleDefinition identifies a bundle definition manifest.
	MediaTypeBundleDefinition = udsoci.MediaTypeBundleDefinition
	// MediaTypeBundleHCL identifies bundle HCL content.
	MediaTypeBundleHCL = udsoci.MediaTypeBundleHCL
	// MediaTypeBundleValuesYAML identifies bundle values content.
	MediaTypeBundleValuesYAML = udsoci.MediaTypeBundleValuesYAML
	// MediaTypeBundle identifies a canonical bundle index.
	MediaTypeBundle = udsoci.MediaTypeBundle
	// AnnotationBundleArchitecture records a bundle index's architecture.
	AnnotationBundleArchitecture = udsoci.AnnotationBundleArchitecture
)

// IsOCIReference reports whether s is an OCI registry reference.
func IsOCIReference(s string) bool { return udsoci.IsOCIReference(s) }

// TrimScheme removes a URI scheme from a reference.
func TrimScheme(s string) string { return udsoci.TrimScheme(s) }

// IsTarZst reports whether s names a tar.zst archive.
func IsTarZst(s string) bool { return artifact.IsTarZst(s) }

// ExtractArtifact extracts and verifies a bundle artifact.
func ExtractArtifact(ctx context.Context, streams iostreams.IOStreams, tarPath, dstDir string) (*artifact.ExtractedBundle, error) {
	return artifact.ExtractArtifact(ctx, streams, tarPath, dstDir)
}

// defaultPuller adapts the internal OCI puller to the public bundle API.
type defaultPuller struct{ puller udsoci.Puller }

// NewDefaultPuller returns the default OCI pull adapter.
func NewDefaultPuller() Puller { return &defaultPuller{puller: udsoci.NewDefaultPuller()} }

// PullBundle pulls a bundle through the internal OCI implementation.
func (p *defaultPuller) PullBundle(ctx context.Context, ref, targetDir string, opts PullOptions) (*PullResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if err := opts.validateBundleVerification(); err != nil {
		return nil, err
	}
	if opts.SkipSignatureVerification {
		WarnSkippedSignatureVerification(opts.Streams)
	}
	result, err := p.puller.PullBundle(ctx, ref, targetDir, toOCIPullOptions(opts, true))
	if result == nil {
		return nil, err
	}
	return &PullResult{OCIReference: result.OCIReference, OutputPath: result.OutputPath}, err
}

// PullPackage pulls a package through the internal OCI implementation.
func (p *defaultPuller) PullPackage(ctx context.Context, ref, targetDir string, opts PullOptions) (*PullResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	result, err := p.puller.PullPackage(ctx, ref, targetDir, toOCIPullOptions(opts, false))
	if result == nil {
		return nil, err
	}
	return &PullResult{OCIReference: result.OCIReference, OutputPath: result.OutputPath}, err
}

// defaultPusher adapts the internal OCI pusher to the public bundle API.
type defaultPusher struct{ pusher udsoci.Pusher }

// NewDefaultPusher returns the default OCI push adapter.
func NewDefaultPusher() Pusher { return &defaultPusher{pusher: udsoci.NewDefaultPusher()} }

// PushBundle pushes a bundle through the internal OCI implementation.
func (p *defaultPusher) PushBundle(ctx context.Context, bundleDir, ref string, opts PushOptions) (*PushResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	result, err := p.pusher.PushBundle(ctx, bundleDir, ref, toOCIPushOptions(opts))
	if result == nil {
		return nil, err
	}
	return &PushResult{OCIReference: result.OCIReference}, err
}

// PushPackage pushes a package through the internal OCI implementation.
func (p *defaultPusher) PushPackage(ctx context.Context, packageDir, ref string, opts PushOptions) (*PushResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	result, err := p.pusher.PushPackage(ctx, packageDir, ref, toOCIPushOptions(opts))
	if result == nil {
		return nil, err
	}
	return &PushResult{OCIReference: result.OCIReference}, err
}

// toOCIPullOptions converts public pull options and hooks to internal equivalents.
func toOCIPullOptions(opts PullOptions, verifyBundle bool) udsoci.PullOptions {
	internal := udsoci.PullOptions{
		Config:                    toInternalConfig(opts.Config),
		Streams:                   opts.Streams,
		SkipSignatureVerification: opts.SkipSignatureVerification,
	}
	internal.PullHooks.CreateBundleArchive = artifact.CreateBundleArchive
	internal.PullHooks.ModifyOrasSettings = opts.PullHooks.ModifyOrasSettings
	if verifyBundle && !opts.SkipSignatureVerification {
		internal.PullHooks.VerifyBundle = func(ctx context.Context, index, evidence []byte) error {
			return verifySignature(ctx, index, evidence, opts.Verification, opts.Config.Options.TmpDir)
		}
	}
	if opts.PullHooks.ToOrasTarget != nil {
		internal.PullHooks.ToOrasTarget = func(ctx context.Context, ref string, _ *udsoci.PullOptions) (oras.Target, error) {
			return opts.PullHooks.ToOrasTarget(ctx, ref, &opts)
		}
	}
	return internal
}

// toOCIPushOptions converts public push options and hooks to internal equivalents.
func toOCIPushOptions(opts PushOptions) udsoci.PushOptions {
	internal := udsoci.PushOptions{Config: toInternalConfig(opts.Config), Streams: opts.Streams}
	internal.PushHooks.ModifyOrasSettings = opts.PushHooks.ModifyOrasSettings
	if opts.PushHooks.ToOrasTarget != nil {
		internal.PushHooks.ToOrasTarget = func(ctx context.Context, ref string, _ *udsoci.PushOptions) (oras.Target, error) {
			return opts.PushHooks.ToOrasTarget(ctx, ref, &opts)
		}
	}
	return internal
}
