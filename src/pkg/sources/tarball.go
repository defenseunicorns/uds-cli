// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package sources contains Zarf packager sources
package sources

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers"
	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/layout"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/packager/filters"
	"github.com/defenseunicorns/zarf/src/pkg/packager/sources"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	av4 "github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
)

// NamespaceOverrideMap is a map of component names to a map of chart names to namespace overrides
type NamespaceOverrideMap = map[string]map[string]string

// TarballBundle is a package source for local tarball bundles that implements Zarf's packager.PackageSource
type TarballBundle struct {
	PkgOpts        *zarfTypes.ZarfPackageOptions
	PkgManifestSHA string
	TmpDir         string
	BundleLocation string
	Pkg            types.Package
	nsOverrides    NamespaceOverrideMap
}

// LoadPackage loads a Zarf package from a local tarball bundle
func (t *TarballBundle) LoadPackage(dst *layout.PackagePaths, filter filters.ComponentFilterStrategy, unarchiveAll bool) (zarfTypes.ZarfPackage, []string, error) {

	packageSpinner := message.NewProgressSpinner("Loading bundled Zarf package: %s", t.Pkg.Name)
	defer packageSpinner.Stop()

	files, err := t.extractPkgFromBundle()
	if err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}

	var pkg zarfTypes.ZarfPackage
	if err = utils.ReadYAMLStrict(dst.ZarfYAML, &pkg); err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}

	// if in dev mode and package is a zarf init config, return an empty package
	if config.Dev && pkg.Kind == zarfTypes.ZarfInitConfig {
		return zarfTypes.ZarfPackage{}, nil, nil
	}

	// filter pkg components and determine if its a partial pkg
	filteredComps, isPartialPkg, err := handleFilter(pkg, filter)
	if err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}
	pkg.Components = filteredComps

	dst.SetFromPaths(files)

	if err := sources.ValidatePackageIntegrity(dst, pkg.Metadata.AggregateChecksum, isPartialPkg); err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}

	if unarchiveAll {
		for _, component := range pkg.Components {
			if err := dst.Components.Unarchive(component); err != nil {
				if layout.IsNotLoaded(err) {
					_, err := dst.Components.Create(component)
					if err != nil {
						return zarfTypes.ZarfPackage{}, nil, err
					}
				} else {
					return zarfTypes.ZarfPackage{}, nil, err
				}
			}
		}

		if dst.SBOMs.Path != "" {
			if err := dst.SBOMs.Unarchive(); err != nil {
				return zarfTypes.ZarfPackage{}, nil, err
			}
		}
	}
	addNamespaceOverrides(&pkg, t.nsOverrides)

	if config.Dev {
		setAsYOLO(&pkg)
	}

	packageSpinner.Successf("Loaded bundled Zarf package: %s", t.Pkg.Name)
	// ensure we're using the correct package name as specified by the bundle
	pkg.Metadata.Name = t.Pkg.Name
	return pkg, nil, err
}

// LoadPackageMetadata loads a Zarf package's metadata from a local tarball bundle
func (t *TarballBundle) LoadPackageMetadata(dst *layout.PackagePaths, _ bool, _ bool) (zarfTypes.ZarfPackage, []string, error) {
	ctx := context.TODO()
	format := av4.CompressedArchive{
		Compression: av4.Zstd{},
		Archival:    av4.Tar{},
	}

	sourceArchive, err := os.Open(t.BundleLocation)
	if err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}

	var imageManifest oci.Manifest
	if err := format.Extract(ctx, sourceArchive, []string{filepath.Join(config.BlobsDir, t.PkgManifestSHA)}, utils.ExtractJSON(&imageManifest)); err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}

	var zarfYamlSHA string
	for _, layer := range imageManifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == config.ZarfYAML {
			zarfYamlSHA = layer.Digest.Encoded()
			break
		}
	}

	if zarfYamlSHA == "" {
		return zarfTypes.ZarfPackage{}, nil, fmt.Errorf(fmt.Sprintf("zarf.yaml with SHA %s not found", zarfYamlSHA))
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
		return zarfTypes.ZarfPackage{}, nil, err
	}

	// grab zarf.yaml and checksums.txt
	filePaths := []string{filepath.Join(config.BlobsDir, zarfYamlSHA), filepath.Join(config.BlobsDir, checksumsSHA)}
	if err := format.Extract(ctx, sourceArchive, filePaths, func(_ context.Context, fileInArchive av4.File) error {
		var fileDst string
		if strings.Contains(fileInArchive.Name(), zarfYamlSHA) {
			fileDst = filepath.Join(dst.Base, config.ZarfYAML)
		} else {
			fileDst = filepath.Join(dst.Base, config.ChecksumsTxt)
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
			return zarfTypes.ZarfPackage{}, nil, err
		}
		return zarfTypes.ZarfPackage{}, nil, err
	}

	// deserialize zarf.yaml to grab checksum for validating pkg integrity
	var pkg zarfTypes.ZarfPackage
	err = utils.ReadYAMLStrict(dst.ZarfYAML, &pkg)
	if err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}

	dst.SetFromPaths(filePaths)
	if err := sources.ValidatePackageIntegrity(dst, pkg.Metadata.AggregateChecksum, true); err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}

	err = sourceArchive.Close()
	// ensure we're using the correct package name as specified by the bundle
	pkg.Metadata.Name = t.Pkg.Name
	return pkg, nil, err
}

// Collect doesn't need to be implemented
func (t *TarballBundle) Collect(_ string) (string, error) {
	return "", fmt.Errorf("not implemented in %T", t)
}

// extractPkgFromBundle extracts a Zarf package from a local tarball bundle
func (t *TarballBundle) extractPkgFromBundle() ([]string, error) {
	var files []string
	format := av4.CompressedArchive{
		Compression: av4.Zstd{},
		Archival:    av4.Tar{},
	}
	sourceArchive, err := os.Open(t.BundleLocation)
	if err != nil {
		return nil, err
	}

	var manifest oci.Manifest
	if err := format.Extract(context.TODO(), sourceArchive, []string{filepath.Join(config.BlobsDir, t.PkgManifestSHA)}, utils.ExtractJSON(&manifest)); err != nil {
		if err := sourceArchive.Close(); err != nil {
			return nil, err
		}
		return nil, err
	}

	if err := sourceArchive.Close(); err != nil {
		return nil, err
	}

	extractLayer := func(_ context.Context, file av4.File) error {
		if file.IsDir() {
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

	var layersToExtract []string

	for _, layer := range manifest.Layers {
		layersToExtract = append(layersToExtract, filepath.Join(config.BlobsDir, layer.Digest.Encoded()))
	}

	sourceArchive, err = os.Open(t.BundleLocation) //reopen to reset reader
	if err != nil {
		return nil, err
	}
	defer sourceArchive.Close()
	err = format.Extract(context.TODO(), sourceArchive, layersToExtract, extractLayer)
	return files, err
}
