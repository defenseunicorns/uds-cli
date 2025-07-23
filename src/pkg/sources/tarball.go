// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package sources contains Zarf packager sources
package sources

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/mholt/archives"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
)

// NamespaceOverrideMap is a map of component names to a map of chart names to namespace overrides
type NamespaceOverrideMap = map[string]map[string]string

// TarballBundle is a package source for local tarball bundles that implements Zarf's packager.PackageSource
type TarballBundle struct {
	PkgOpts                 *zarfTypes.ZarfPackageOptions
	PkgManifestSHA          string
	TmpDir                  string
	BundleLocation          string
	Pkg                     types.Package
	PublicKeyPath           string
	nsOverrides             NamespaceOverrideMap
	SkipSignatureValidation bool
}

// LoadPackage loads a Zarf package from a local tarball bundle
func (t *TarballBundle) LoadPackage(ctx context.Context, filter filters.ComponentFilterStrategy) (*layout.PackageLayout, []string, error) {
	packageSpinner := message.NewProgressSpinner("Loading bundled Zarf package: %s", t.Pkg.Name)
	defer packageSpinner.Stop()

	_, err := t.extractPkgFromBundle()
	if err != nil {
		return nil, nil, err
	}

	var pkg v1alpha1.ZarfPackage
	if err = utils.ReadYAMLStrict(filepath.Join(t.TmpDir, layout.ZarfYAML), &pkg); err != nil {
		return nil, nil, err
	}

	// if in dev mode and package is a zarf init config, return an empty package
	if config.Dev && pkg.Kind == v1alpha1.ZarfInitConfig {
		return nil, nil, nil
	}

	// filter pkg components and determine if its a partial pkg
	filteredComps, isPartialPkg, err := handleFilter(pkg, filter)
	if err != nil {
		return nil, nil, err
	}
	pkg.Components = filteredComps

	layoutOpts := layout.PackageLayoutOptions{
		PublicKeyPath:           t.PublicKeyPath,
		SkipSignatureValidation: t.SkipSignatureValidation,
		IsPartial:               isPartialPkg,
		Filter:                  filter,
	}

	pkgLayout, err := layout.LoadFromDir(ctx, t.TmpDir, layoutOpts)
	if err != nil {
		return nil, nil, err
	}

	addNamespaceOverrides(&pkgLayout.Pkg, t.nsOverrides)

	if config.Dev {
		setAsYOLO(&pkgLayout.Pkg)
	}

	packageSpinner.Successf("Loaded bundled Zarf package: %s", t.Pkg.Name)
	// ensure we're using the correct package name as specified by the bundle
	pkgLayout.Pkg.Metadata.Name = t.Pkg.Name
	return pkgLayout, nil, err
}

// LoadPackageMetadata loads a Zarf package's metadata from a local tarball bundle
func (t *TarballBundle) LoadPackageMetadata(_ context.Context, _ bool, _ bool) (v1alpha1.ZarfPackage, []string, error) {
	ctx := context.TODO()

	sourceArchive, err := os.Open(t.BundleLocation)
	if err != nil {
		return v1alpha1.ZarfPackage{}, nil, err
	}

	var imageManifest oci.Manifest
	if err := config.BundleArchiveFormat.Extract(ctx, sourceArchive, utils.ExtractJSON(&imageManifest, filepath.Join(config.BlobsDir, t.PkgManifestSHA))); err != nil {
		return v1alpha1.ZarfPackage{}, nil, err
	}

	var zarfYamlSHA string
	for _, layer := range imageManifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == layout.ZarfYAML {
			zarfYamlSHA = layer.Digest.Encoded()
			break
		}
	}

	if zarfYamlSHA == "" {
		return v1alpha1.ZarfPackage{}, nil, fmt.Errorf("zarf.yaml with SHA %s not found", zarfYamlSHA)
	}

	// grab SHA of checksums.txt
	var checksumsSHA string
	for _, layer := range imageManifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == config.ChecksumsTxt {
			checksumsSHA = layer.Digest.Encoded()
			break
		}
	}

	// reset file reader
	_, err = sourceArchive.Seek(0, 0)
	if err != nil {
		return v1alpha1.ZarfPackage{}, nil, err
	}

	// grab zarf.yaml and checksums.txt
	filePaths := []string{filepath.Join(config.BlobsDir, zarfYamlSHA), filepath.Join(config.BlobsDir, checksumsSHA)}
	if err := config.BundleArchiveFormat.Extract(ctx, sourceArchive, func(_ context.Context, fileInArchive archives.FileInfo) error {
		if !slices.Contains(filePaths, fileInArchive.NameInArchive) {
			return nil
		}

		var fileDst string
		if strings.Contains(fileInArchive.Name(), zarfYamlSHA) {
			fileDst = filepath.Join(t.TmpDir, layout.ZarfYAML)
		} else {
			fileDst = filepath.Join(t.TmpDir, layout.Checksums)
		}
		outFile, err := os.Create(fileDst)
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
		err = sourceArchive.Close()
		if err != nil {
			return v1alpha1.ZarfPackage{}, nil, err
		}
		return v1alpha1.ZarfPackage{}, nil, err
	}

	// deserialize zarf.yaml to grab checksum for validating pkg integrity
	var pkg v1alpha1.ZarfPackage
	err = utils.ReadYAMLStrict(filepath.Join(t.TmpDir, layout.ZarfYAML), &pkg)
	if err != nil {
		return v1alpha1.ZarfPackage{}, nil, err
	}

	err = sourceArchive.Close()
	// ensure we're using the correct package name as specified by the bundle
	pkg.Metadata.Name = t.Pkg.Name
	return pkg, nil, err
}

// Collect doesn't need to be implemented
func (t *TarballBundle) Collect(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("not implemented in %T", t)
}

// extractPkgFromBundle extracts a Zarf package from a local tarball bundle
func (t *TarballBundle) extractPkgFromBundle() ([]string, error) {
	var files []string
	sourceArchive, err := os.Open(t.BundleLocation)
	if err != nil {
		return nil, err
	}

	var manifest oci.Manifest
	if err := config.BundleArchiveFormat.Extract(context.TODO(), sourceArchive, utils.ExtractJSON(&manifest, filepath.Join(config.BlobsDir, t.PkgManifestSHA))); err != nil {
		if err := sourceArchive.Close(); err != nil {
			return nil, err
		}
		return nil, err
	}

	if err := sourceArchive.Close(); err != nil {
		return nil, err
	}
	var layersToExtract []string
	for _, layer := range manifest.Layers {
		layersToExtract = append(layersToExtract, filepath.Join(config.BlobsDir, layer.Digest.Encoded()))
	}
	extractLayer := func(_ context.Context, file archives.FileInfo) error {
		if file.IsDir() || !slices.Contains(layersToExtract, file.NameInArchive) {
			return nil
		}
		stream, err := file.Open()
		if err != nil {
			return err
		}
		defer stream.Close()

		desc := helpers.Find(manifest.Layers, func(layer ocispec.Descriptor) bool {
			return layer.Digest.Encoded() == filepath.Base(file.NameInArchive)
		})

		path := desc.Annotations[ocispec.AnnotationTitle]
		cleanPath := filepath.Clean(path)
		if strings.Contains(cleanPath, "..") {
			// throw an error for dangerous looking paths
			return fmt.Errorf("invalid path detected: %s", path)
		}
		size := desc.Size
		layerDst := filepath.Join(t.TmpDir, cleanPath)
		if err := helpers.CreateDirectory(filepath.Dir(layerDst), 0700); err != nil {
			return err
		}

		target, err := os.Create(layerDst)
		if err != nil {
			return err
		}
		defer target.Close()

		written, err := io.Copy(target, stream)
		if err != nil {
			return err
		}
		if written != size {
			return fmt.Errorf("expected to write %d bytes to %s, wrote %d", size, path, written)
		}

		files = append(files, strings.ReplaceAll(layerDst, t.TmpDir+"/", ""))
		return nil
	}

	sourceArchive, err = os.Open(t.BundleLocation) //reopen to reset reader
	if err != nil {
		return nil, err
	}
	defer sourceArchive.Close()
	err = config.BundleArchiveFormat.Extract(context.TODO(), sourceArchive, extractLayer)
	return files, err
}
