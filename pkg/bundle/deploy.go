// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle implements UDS bundle deployment functionality.
//
// Below is an illustration of the data flow
//
//	bundle.uds.hcl ──▶ HCLParser ──▶ UDSBundle ──▶ Validate ──▶ DAG ──▶ Levels ──▶ Deploy(opts)
//	                                                              UDSBundleConfig (pre-resolved) ──┘
package bundle

import (
	"context"
	"fmt"
	"os"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Deploy deploys a UDS bundle to a Kubernetes cluster.
// It validates the bundle and delegates the bundle-level deployment (DAG
// traversal, ordering, parallelism, concurrency limits) to the Deployer
// implementation.
//
// When opts.Bundle is set, bundle file parsing is skipped; the provided
// Bundle is still re-validated before deployment.
func Deploy(ctx context.Context, opts DeployOptions) (*DeployResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	s := logger.Bind(opts.Streams, opts.Config.Global.LogLevel)

	b := opts.Bundle
	if b != nil {
		if err := b.Validate(); err != nil {
			return nil, fmt.Errorf("bundle validation failed: %w", err)
		}
	}
	if b == nil {
		s.Debug("parsing bundle", "path", opts.BundlePath)
		parser := NewHCLParser(opts.Config.Options.Architecture, s)
		var err error
		b, err = parser.ParseBundleFile(ctx, opts.BundlePath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse bundle: %w", err)
		}
		s.Debug("bundle parsed", "name", b.Metadata.Name, "packages", len(b.Packages))

		if err := b.Validate(); err != nil {
			return nil, fmt.Errorf("bundle validation failed: %w", err)
		}
		s.Debug("bundle validated")
	}
	if !opts.Force {
		if err := ValidateDeploySafety(ctx, s, b, opts.Packages); err != nil {
			return nil, err
		}
	}

	if opts.Source != nil && opts.Source.ValuesFilesOverride != nil {
		artifact.ApplyValuesFilesOverride(b.Packages, opts.Source.ValuesFilesOverride)
	}

	var loader PackageLayoutLoader
	if opts.Source != nil {
		loader = opts.Source.Loader
	}

	deployer := NewZarfDeployer(s, loader)
	return deployer.DeployBundle(ctx, b, opts)
}

// PrepareDeploySource initializes a DeploySource from either a bundle source
// directory or a .tar.zst bundle artifact path. tmpDir is the directory under
// which any temporary files or directories should be created. If empty, the
// system default temp directory will be used.
func PrepareDeploySource(ctx context.Context, streams iostreams.IOStreams, path, tmpDir string) (*DeploySource, error) {
	if path == "" {
		return nil, fmt.Errorf("path must not be empty")
	}
	if artifact.IsTarZst(path) {
		return prepareExtractedArtifactSource(ctx, streams, path, tmpDir)
	}
	return prepareDirectorySource(path), nil
}

// prepareExtractedArtifactSource extracts an artifact into an owned temporary workspace.
func prepareExtractedArtifactSource(ctx context.Context, streams iostreams.IOStreams, path, tmpDir string) (*DeploySource, error) {
	workspaceDir, err := os.MkdirTemp(tmpDir, "uds-bundle-deploy-*")
	if err != nil {
		return nil, fmt.Errorf("creating workspace for bundle artifact: %w", err)
	}

	extracted, err := artifact.ExtractArtifact(ctx, streams, path, workspaceDir)
	if err != nil {
		_ = os.RemoveAll(workspaceDir)
		return nil, fmt.Errorf("extracting bundle artifact: %w", err)
	}

	valuesOverride, err := extracted.ValuesFilesByPackage()
	if err != nil {
		_ = os.RemoveAll(workspaceDir)
		return nil, fmt.Errorf("collecting values files from artifact: %w", err)
	}

	return &DeploySource{
		BundlePath: extracted.BundleDefPath,
		Loader: &ExtractedArtifactPackageLayoutLoader{
			OCIDir:           extracted.OCIDir,
			PackageDigests:   descriptorDigests(extracted.PackageManifests),
			packageManifests: extracted.PackageManifests,
		},
		ValuesFilesOverride: valuesOverride,
		closer:              tempDirCloser{path: workspaceDir},
	}, nil
}

func descriptorDigests(manifests map[string]ocispec.Descriptor) map[string]string {
	digests := make(map[string]string, len(manifests))
	for ref, descriptor := range manifests {
		digests[ref] = descriptor.Digest.String()
	}
	return digests
}

// prepareDirectorySource prepares a deploy source backed by a local bundle directory.
func prepareDirectorySource(path string) *DeploySource {
	return &DeploySource{
		BundlePath: ResolveBundlePath(path),
	}
}

// tempDirCloser removes an owned temporary workspace when closed.
type tempDirCloser struct {
	path string
}

// Close removes the temporary directory.
func (c tempDirCloser) Close() error {
	return os.RemoveAll(c.path)
}
