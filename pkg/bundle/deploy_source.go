// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"
)

// PrepareDeploySource initializes a DeploySource from either a bundle source
// directory or a .tar.zst bundle artifact path. tmpDir is the directory under
// which any temporary files or directories should be created. If empty, the
// system default temp directory will be used.
func PrepareDeploySource(ctx context.Context, path, tmpDir string) (*DeploySource, error) {
	if IsTarZst(path) {
		return prepareExtractedArtifactSource(ctx, path, tmpDir)
	}
	return prepareDirectorySource(path), nil
}

func prepareExtractedArtifactSource(ctx context.Context, path, tmpDir string) (*DeploySource, error) {
	workspaceDir, err := os.MkdirTemp(tmpDir, "uds-bundle-deploy-*")
	if err != nil {
		return nil, fmt.Errorf("creating workspace for bundle artifact: %w", err)
	}

	extracted, err := ExtractArtifact(ctx, path, workspaceDir)
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
			OCIDir:         extracted.OCIDir,
			PackageDigests: extracted.PackageDigests,
		},
		ValuesFilesOverride: valuesOverride,
		closer:              tempDirCloser{path: workspaceDir},
	}, nil
}

func prepareDirectorySource(path string) *DeploySource {
	return &DeploySource{
		BundlePath: ResolveBundlePath(path),
	}
}

type tempDirCloser struct {
	path string
}

func (c tempDirCloser) Close() error {
	return os.RemoveAll(c.path)
}
