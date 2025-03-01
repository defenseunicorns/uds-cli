// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package fetcher contains functionality to fetch local and remote Zarf pkgs for local bundling
package fetcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	zarfCmd "github.com/zarf-dev/zarf/src/cmd"
	zarfCfg "github.com/zarf-dev/zarf/src/config"
	"github.com/zarf-dev/zarf/src/pkg/message"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
)

// remoteFetcher fetches remote Zarf pkgs for local bundles
type remoteFetcher struct {
	pkg             types.Package
	cfg             Config
	pkgRootManifest *oci.Manifest
	remote          *zoci.Remote
}

// Fetch fetches a Zarf pkg and puts it into a local bundle
func (f *remoteFetcher) Fetch() ([]ocispec.Descriptor, error) {
	fetchSpinner := message.NewProgressSpinner("Fetching package %s", f.pkg.Name)
	defer fetchSpinner.Stop()

	// Pull the remote package local tmpdir
	zarfCmd := zarfCmd.NewZarfCommand()
	zarfCfg.CLIArch = config.GetArch()
	ociURL := "oci://" + f.pkg.Repository + ":" + f.pkg.Ref
	outFlag := "--output-directory=" + f.cfg.TmpDstDir
	cmdArgs := []string{
		"package",
		"pull",
		ociURL,
		outFlag,
	}

	// Add path to public key if provided
	var err error
	f.pkg.PublicKey, err = getAbsKeyPath(f.pkg.PublicKey, f.cfg.CreateSrcDir)
	if err != nil {
		return nil, err
	} else if f.pkg.PublicKey != "" {
		cmdArgs = append(cmdArgs, "--key="+f.pkg.PublicKey)
	}

	zarfCmd.SetArgs(cmdArgs)
	err = zarfCmd.Execute()
	if err != nil {
		return []ocispec.Descriptor{}, err
	}

	// Fetch the descriptor layers from the, now local, zarf package
	localFetcher := localFetcher{
		pkg: f.pkg,
		cfg: f.cfg,
	}
	zarfPkgName := fmt.Sprintf("zarf-package-%s-%s-%s.tar.zst", f.pkg.Name, config.GetArch(), f.pkg.Ref)
	if _, err := os.Stat(filepath.Join(f.cfg.TmpDstDir, zarfPkgName)); err != nil {
		// the downloaded packge might have been an 'init' package with a different file name
		zarfPkgName = fmt.Sprintf("zarf-%s-%s-%s.tar.zst", f.pkg.Name, config.GetArch(), f.pkg.Ref)
		if _, err := os.Stat(filepath.Join(f.cfg.TmpDstDir, zarfPkgName)); err != nil {
			return nil, fmt.Errorf("Unable to fetch upstream package %s", ociURL)
		}
	}
	localFetcher.pkg.Path = filepath.Join(f.cfg.TmpDstDir, zarfPkgName)

	return localFetcher.Fetch()
}

func (f *remoteFetcher) GetPkgMetadata() (v1alpha1.ZarfPackage, error) {
	ctx := context.TODO()
	platform := ocispec.Platform{
		Architecture: config.GetArch(),
		OS:           oci.MultiOS,
	}

	// create OCI remote
	url := fmt.Sprintf("%s:%s", f.pkg.Repository, f.pkg.Ref)
	remote, err := zoci.NewRemote(ctx, url, platform)
	if err != nil {
		return v1alpha1.ZarfPackage{}, err
	}

	// get package metadata
	tmpDir, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
	if err != nil {
		return v1alpha1.ZarfPackage{}, fmt.Errorf("bundler unable to create temp directory: %w", err)
	}
	if _, err := remote.PullPackageMetadata(ctx, tmpDir); err != nil {
		return v1alpha1.ZarfPackage{}, err
	}

	// read metadata
	zarfYAML := v1alpha1.ZarfPackage{}
	zarfYAMLPath := filepath.Join(tmpDir, config.ZarfYAML)
	err = utils.ReadYAMLStrict(zarfYAMLPath, &zarfYAML)
	if err != nil {
		return v1alpha1.ZarfPackage{}, err
	}
	return zarfYAML, err
}
