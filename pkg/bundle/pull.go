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

// PullOptions holds configuration for pulling a bundle from an OCI registry.
type PullOptions struct {
	Config                    *UDSBundleConfig
	Verification              VerificationPolicy
	SkipSignatureVerification bool
	Streams                   iostreams.IOStreams
}

// PullResult represents the output of a bundle pull operation.
type PullResult struct {
	OCIReference string `json:"ociReference" yaml:"ociReference" text:"OCI Reference"`
	OutputPath   string `json:"outputPath" yaml:"outputPath" text:"Output Path"`
}

// Pull pulls a bundle artifact from an OCI registry into targetDir.
func Pull(ctx context.Context, ref, targetDir string, opts PullOptions) (*PullResult, error) {
	if !opts.SkipSignatureVerification {
		if err := opts.Verification.Validate(); err != nil {
			return nil, err
		}
	}
	return PullBundle(ctx, ref, targetDir, opts)
}

type pullHooks struct {
	toOrasTarget       func(ctx context.Context, ociReference string, opts *PullOptions) (oras.Target, error)
	modifyOrasSettings func(ctx context.Context, copyOptions *oras.CopyOptions) error
}

// PullBundle pulls a bundle artifact from OCI storage into targetDir.
func PullBundle(ctx context.Context, ref, targetDir string, opts PullOptions) (*PullResult, error) {
	return pullBundle(ctx, ref, targetDir, opts, pullHooks{})
}

func pullBundle(ctx context.Context, ref, targetDir string, opts PullOptions, hooks pullHooks) (*PullResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	result, err := udsoci.NewDefaultPuller().PullBundle(ctx, ref, targetDir, toOCIPullOptions(opts, hooks))
	if result == nil {
		return nil, err
	}
	return &PullResult{OCIReference: result.OCIReference, OutputPath: result.OutputPath}, err
}

// toOCIPullOptions converts public pull options and hooks to internal equivalents.
func toOCIPullOptions(opts PullOptions, hooks pullHooks) udsoci.PullOptions {
	internal := udsoci.PullOptions{
		Config:                    toInternalConfig(opts.Config),
		Streams:                   opts.Streams,
		SkipSignatureVerification: opts.SkipSignatureVerification,
	}
	internal.PullHooks.CreateBundleArchive = artifact.CreateBundleArchive
	internal.PullHooks.ModifyOrasSettings = hooks.modifyOrasSettings
	if !opts.SkipSignatureVerification && opts.Verification.configured() {
		internal.PullHooks.VerifyBundle = func(ctx context.Context, index, evidence []byte) error {
			return verifySignature(ctx, index, evidence, opts.Verification, opts.Config.Options.TmpDir)
		}
	}
	if hooks.toOrasTarget != nil {
		internal.PullHooks.ToOrasTarget = func(ctx context.Context, ref string, _ *udsoci.PullOptions) (oras.Target, error) {
			return hooks.toOrasTarget(ctx, ref, &opts)
		}
	}
	return internal
}
