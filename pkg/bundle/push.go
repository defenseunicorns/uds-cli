// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	oras "oras.land/oras-go/v2"
)

// Push pushes a local bundle tarball to an OCI registry.
func Push(ctx context.Context, bundleTarball, ref string, opts PushOptions) (*PushResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	s := logger.Bind(opts.Streams, opts.Config.Options.LogLevel)
	signatureEntries, err := artifact.CountTarZstEntries(ctx, bundleTarball, bundleSignatureFileName)
	if err != nil {
		return nil, fmt.Errorf("checking bundle signature evidence: %w", err)
	}
	if signatureEntries > 1 {
		return nil, fmt.Errorf("expected exactly one bundle signature evidence entry, found %d", signatureEntries)
	}
	tmp, err := os.MkdirTemp(opts.Config.Options.TmpDir, "uds-bundle-push-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	s.Info("extracting bundle archive", "source", bundleTarball)
	s.Debug("extracting bundle archive", "source", bundleTarball, "output", tmp)
	if err := artifact.ExtractTarZst(ctx, s, bundleTarball, tmp); err != nil {
		return nil, fmt.Errorf("extracting bundle: %w", err)
	}
	return pushBundle(ctx, tmp, ref, opts, pushHooks{})
}

// PushOptions holds configuration for pushing a bundle to an OCI registry.
type PushOptions struct {
	Config  *UDSBundleConfig
	Streams iostreams.IOStreams
}

// PushResult represents the output of a bundle push operation.
type PushResult struct {
	OCIReference string `json:"ociReference" yaml:"ociReference" text:"OCI Reference"`
}

type pushHooks struct {
	toOrasTarget       func(ctx context.Context, ociReference string, opts *PushOptions) (oras.Target, error)
	modifyOrasSettings func(ctx context.Context, copyOptions *oras.CopyOptions) error
}

func pushBundle(ctx context.Context, bundleDir, ref string, opts PushOptions, hooks pushHooks) (*PushResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	result, err := udsoci.NewDefaultPusher().PushBundle(ctx, bundleDir, ref, toOCIPushOptions(opts, hooks))
	if result == nil {
		return nil, err
	}
	return &PushResult{OCIReference: result.OCIReference}, err
}

// toOCIPushOptions converts public push options and hooks to internal equivalents.
func toOCIPushOptions(opts PushOptions, hooks pushHooks) udsoci.PushOptions {
	internal := udsoci.PushOptions{Config: toInternalConfig(opts.Config), Streams: opts.Streams}
	internal.PushHooks.ModifyOrasSettings = hooks.modifyOrasSettings
	if hooks.toOrasTarget != nil {
		internal.PushHooks.ToOrasTarget = func(ctx context.Context, ref string, _ *udsoci.PushOptions) (oras.Target, error) {
			return hooks.toOrasTarget(ctx, ref, &opts)
		}
	}
	return internal
}
