// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package fetcher contains functionality to fetch local and remote Zarf pkgs for local bundling
package fetcher

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils/boci"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/mholt/archives"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"
)

type localFetcher struct {
	pkg types.Package
	cfg Config
}

// Fetch fetches a local Zarf pkg and puts it into a local bundle
func (f *localFetcher) Fetch() ([]ocispec.Descriptor, error) {
	fetchSpinner := message.NewProgressSpinner("Fetching package %s", f.pkg.Name)
	defer fetchSpinner.Stop()

	layerDescs, err := f.toBundle()
	if err != nil {
		return nil, err
	}
	fetchSpinner.Successf("Fetched package: %s", f.pkg.Name)
	return layerDescs, nil
}

// GetPkgMetadata grabs metadata from a local Zarf package's zarf.yaml
func (f *localFetcher) GetPkgMetadata() (v1alpha1.ZarfPackage, error) {
	// todo: can we refactor to use Zarf fns?
	tmpDir, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
	if err != nil {
		return v1alpha1.ZarfPackage{}, err
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck

	zarfTarball, err := os.Open(f.cfg.Bundle.Packages[f.cfg.PkgIter].Path)
	if err != nil {
		return v1alpha1.ZarfPackage{}, err
	}
	if err := config.BundleArchiveFormat.Extract(context.TODO(), zarfTarball, func(_ context.Context, fileInArchive archives.FileInfo) error {
		if fileInArchive.NameInArchive != config.ZarfYAML {
			return nil
		}
		// write zarf.yaml to tmp for checking optional components later on
		dst := filepath.Join(tmpDir, fileInArchive.NameInArchive)
		outFile, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer outFile.Close()
		stream, err := fileInArchive.Open()
		if err != nil {
			return err
		}
		defer stream.Close()
		_, err = io.Copy(outFile, io.Reader(stream))
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		zarfTarball.Close()
		return v1alpha1.ZarfPackage{}, err
	}
	zarfYAML := v1alpha1.ZarfPackage{}
	zarfYAMLPath := filepath.Join(tmpDir, config.ZarfYAML)
	err = utils.ReadYAMLStrict(zarfYAMLPath, &zarfYAML)
	if err != nil {
		return v1alpha1.ZarfPackage{}, err
	}
	return zarfYAML, err
}

// toBundle transfers a Zarf package to a given Bundle
func (f *localFetcher) toBundle() ([]ocispec.Descriptor, error) {
	ctx := context.TODO()

	tmpDir, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck

	verifyOpts, err := utils.BuildVerifyBlobOptions(f.pkg, tmpDir)
	if err != nil {
		return nil, err
	}

	filter := filters.Combine(
		filters.ForDeploy(strings.Join(f.pkg.OptionalComponents, ","), false),
	)

	remoteOpts := zarfTypes.RemoteOptions{
		PlainHTTP:             config.CommonOptions.Insecure,
		InsecureSkipTLSVerify: config.CommonOptions.Insecure,
	}

	loadOpts := packager.LoadOptions{
		Filter:               filter,
		CachePath:            config.CommonOptions.CachePath,
		VerifyBlobOptions:    verifyOpts,
		RemoteOptions:        remoteOpts,
		VerificationStrategy: utils.GetPackageVerificationStrategy(f.cfg.SkipSignatureValidation),
		OCIConcurrency:       config.CommonOptions.OCIConcurrency,
	}

	pkgLayout, err := utils.LoadPackage(ctx, f.pkg.Path, loadOpts)
	if err != nil {
		return nil, err
	}

	rootManifestDesc, err := pkgLayout.Resolve(ctx, pkgLayout.Digest())
	if err != nil {
		return nil, err
	}
	rootManifest, err := pkgLayout.Manifest()
	if err != nil {
		return nil, fmt.Errorf("reading package manifest for %q: %w", f.pkg.Name, err)
	}
	descs, err := boci.SelectPackageContent(ctx, rootManifest, pkgLayout, f.pkg.OptionalComponents)
	if err != nil {
		return nil, fmt.Errorf("selecting layers for package %q: %w", f.pkg.Name, err)
	}
	for _, desc := range descs {
		if err := copyPackageBlob(ctx, pkgLayout, f.cfg.Store, desc); err != nil {
			return nil, err
		}
	}

	if err := copyPackageBlob(ctx, pkgLayout, f.cfg.Store, rootManifestDesc); err != nil {
		return nil, err
	}
	descs = append(descs, rootManifestDesc)

	f.cfg.Bundle.Packages[f.cfg.PkgIter].Ref += "@" + rootManifestDesc.Digest.String()

	manifestLayerDesc := packageManifestLayerDescriptor(rootManifestDesc)
	f.cfg.BundleRootManifest.Layers = append(f.cfg.BundleRootManifest.Layers, manifestLayerDesc)
	return descs, nil
}

func copyPackageBlob(ctx context.Context, source content.Fetcher, target *ocistore.Store, desc ocispec.Descriptor) error {
	exists, err := target.Exists(ctx, desc)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	blob, err := source.Fetch(ctx, desc)
	if err != nil {
		return err
	}
	defer blob.Close()
	return target.Push(ctx, desc, blob)
}
