// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	goyaml "github.com/goccy/go-yaml"
	"github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	ocistore "oras.land/oras-go/v2/content/oci"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
)

type ociProvider struct {
	ctx context.Context
	src string
	dst string
	*oci.OrasRemote
	manifest *oci.ZarfOCIManifest
}

func (op *ociProvider) getBundleManifest() error {
	if op.manifest != nil {
		return nil
	}
	root, err := op.FetchRoot()
	if err != nil {
		return err
	}
	op.manifest = root
	return nil
}

// LoadPackage loads a package from a remote bundle
func (op *ociProvider) LoadPackage(sha, destinationDir string, _ int) (PathMap, error) {
	// todo: use oras.Copy for faster downloads
	if destinationDir == op.dst {
		return nil, fmt.Errorf("destination directory cannot be the same as the bundle directory")
	}

	if err := op.getBundleManifest(); err != nil {
		return nil, err
	}
	pkgManifestDesc := op.manifest.Locate(sha)
	if oci.IsEmptyDescriptor(pkgManifestDesc) {
		return nil, fmt.Errorf("package %s does not exist in this bundle", sha)
	}
	// hack to Zarf media type so that FetchManifest works
	pkgManifestDesc.MediaType = oci.ZarfLayerMediaTypeBlob
	pkgManifest, err := op.FetchManifest(pkgManifestDesc)
	if err != nil || pkgManifest == nil {
		return nil, err
	}

	// including the package manifest uses some ORAs FindSuccessors hackery to expand the manifest into all layers
	// as oras.Copy was designed for resolving layers via a manifest reference, not a manifest embedded inside of another
	// image
	layersToPull := []ocispec.Descriptor{pkgManifestDesc}
	for _, layer := range pkgManifest.Layers {
		// only fetch layers that exist
		// since optional-components exists, there will be layers that don't exist
		// as the package's preserved manifest will contain all layers for all components
		ok, _ := op.Repo().Blobs().Exists(op.ctx, layer)
		if ok {
			layersToPull = append(layersToPull, layer)
		}
	}

	store, err := file.New(destinationDir)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	spinner := message.NewProgressSpinner("Pulling bundled Zarf package")
	defer spinner.Stop()
	for _, layer := range layersToPull {
		spinner.Updatef(fmt.Sprintf("Pulling bundle layer: %s", layer.Digest.Encoded()))
		lb, err := op.Repo().Fetch(op.ctx, layer)
		if err != nil {
			return nil, err
		}

		err = store.Push(op.ctx, layer, lb)
		if err != nil {
			return nil, err
		}
	}

	spinner.Successf("Package pull successful")

	loaded := make(PathMap)
	for _, layer := range layersToPull {
		rel := layer.Annotations[ocispec.AnnotationTitle]
		loaded[rel] = filepath.Join(destinationDir, rel)
	}
	return loaded, nil
}

// LoadBundleMetadata loads a remote bundle's metadata
func (op *ociProvider) LoadBundleMetadata() (PathMap, error) {
	if err := zarfUtils.CreateDirectory(filepath.Join(op.dst, config.BlobsDir), 0700); err != nil {
		return nil, err
	}
	layers, err := op.PullPackagePaths(config.BundleAlwaysPull, filepath.Join(op.dst, config.BlobsDir))
	if err != nil {
		return nil, err
	}

	loaded := make(PathMap)
	for _, layer := range layers {
		rel := layer.Annotations[ocispec.AnnotationTitle]
		abs := filepath.Join(op.dst, config.BlobsDir, rel)
		absSha := filepath.Join(op.dst, config.BlobsDir, layer.Digest.Encoded())
		if err := os.Rename(abs, absSha); err != nil {
			return nil, err
		}
		loaded[rel] = absSha
	}
	op.getBundleManifest()
	return loaded, nil
}

// CreateBundleSBOM creates a bundle-level SBOM from the underlying Zarf packages, if the Zarf package contains an SBOM
func (op *ociProvider) CreateBundleSBOM(extractSBOM bool) error {
	SBOMArtifactPathMap := make(PathMap)
	root, err := op.FetchRoot()
	if err != nil {
		return err
	}
	// make tmp dir for pkg SBOM extraction
	err = os.Mkdir(filepath.Join(op.dst, config.BundleSBOM), 0700)
	if err != nil {
		return err
	}
	// iterate through Zarf image manifests and find the Zarf pkg's sboms.tar
	for _, layer := range root.Layers {
		zarfManifest, err := op.OrasRemote.FetchManifest(layer)
		if err != nil {
			continue
		}
		// read in and unmarshal Zarf image manifest
		sbomDesc := zarfManifest.Locate(config.SBOMsTar)
		zarfYAML, err := op.OrasRemote.FetchZarfYAML(zarfManifest)
		if err != nil {
			return err
		}
		if sbomDesc.Annotations == nil {
			message.Warnf("%s not found in Zarf pkg: %s", config.SBOMsTar, zarfYAML.Metadata.Name)
		}
		// grab sboms.tar and extract
		sbomBytes, err := op.OrasRemote.FetchLayer(sbomDesc)
		if err != nil {
			return err
		}
		extractor := utils.SBOMExtractor(op.dst, SBOMArtifactPathMap)
		err = archiver.Tar{}.Extract(context.TODO(), bytes.NewReader(sbomBytes), nil, extractor)
		if err != nil {
			return err
		}
	}
	if extractSBOM {
		currentDir, err := os.Getwd()
		if err != nil {
			return err
		}
		err = utils.MoveExtractedSBOMs(op.dst, currentDir)
		if err != nil {
			return err
		}
	} else {
		err = utils.CreateSBOMArtifact(SBOMArtifactPathMap)
		if err != nil {
			return err
		}
	}
	return nil
}

// LoadBundle loads a bundle from a remote source
func (op *ociProvider) LoadBundle(_ int) (PathMap, error) {
	var layersToPull []ocispec.Descriptor

	if err := op.getBundleManifest(); err != nil {
		return nil, err
	}

	loaded, err := op.LoadBundleMetadata() // todo: remove? this seems redundant, can we pass the "loaded" var in
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(loaded[config.BundleYAML])
	if err != nil {
		return nil, err
	}

	var bundle types.UDSBundle
	if err := goyaml.Unmarshal(b, &bundle); err != nil {
		return nil, err
	}

	for _, pkg := range bundle.ZarfPackages {
		sha := strings.Split(pkg.Ref, "@sha256:")[1] // this is where we use the SHA appended to the Zarf pkg inside the bundle
		manifestDesc := op.manifest.Locate(sha)
		if err != nil {
			return nil, err
		}
		manifestBytes, err := op.FetchLayer(manifestDesc)
		if err != nil {
			return nil, err
		}
		var manifest oci.ZarfOCIManifest
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			return nil, err
		}
		layersToPull = append(layersToPull, manifestDesc)
		progressBar := message.NewProgressBar(int64(len(manifest.Layers)), fmt.Sprintf("Verifying layers in Zarf package: %s", pkg.Name))
		for _, layer := range manifest.Layers {
			ok, err := op.Repo().Blobs().Exists(op.ctx, layer)
			progressBar.Add(1)
			if err != nil {
				return nil, err
			}
			if ok {
				layersToPull = append(layersToPull, layer)
			}
		}
		progressBar.Successf("Verified %s package", pkg.Name)
	}

	store, err := ocistore.NewWithContext(op.ctx, op.dst)
	if err != nil {
		return nil, err
	}

	rootDesc, err := op.ResolveRoot()
	if err != nil {
		return nil, err
	}
	layersToPull = append(layersToPull, rootDesc)

	// copy bundle
	copyOpts, estimatedBytes, err := utils.CreateCopyOpts(layersToPull, config.CommonOptions.OCIConcurrency)
	if err != nil {
		return nil, err
	}

	// Create a thread to update a progress bar as we save the package to disk
	doneSaving := make(chan int)
	var wg sync.WaitGroup
	wg.Add(1)
	go zarfUtils.RenderProgressBarForLocalDirWrite(op.dst, estimatedBytes, &wg, doneSaving, fmt.Sprintf("Pulling bundle: %s", bundle.Metadata.Name))
	_, err = oras.Copy(op.ctx, op.Repo(), op.Repo().Reference.String(), store, op.Repo().Reference.String(), copyOpts)
	if err != nil {
		doneSaving <- 1
		return nil, err
	}

	doneSaving <- 1
	wg.Wait()

	for _, layer := range layersToPull {
		sha := layer.Digest.Encoded()
		loaded[sha] = filepath.Join(op.dst, config.BlobsDir, sha)
	}

	return loaded, nil
}

func (op *ociProvider) PublishBundle(_ types.UDSBundle, _ *oci.OrasRemote) error {
	// todo: implement moving bundles from one registry to another
	message.Warnf("moving bundles in between remote registries not yet supported")
	return nil
}
