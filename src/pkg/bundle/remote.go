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
	"slices"
	"strings"
	"sync"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/cluster"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	goyaml "github.com/goccy/go-yaml"
	"github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	ocistore "oras.land/oras-go/v2/content/oci"
)

const (
	// GHCRPackagesPath is the default package path
	GHCRPackagesPath = "oci://ghcr.io/defenseunicorns/packages/"
	// GHCRUDSBundlePath is the default path for uds bundles
	GHCRUDSBundlePath = GHCRPackagesPath + "uds/bundles/"
	// GHCRDeliveryBundlePath is the default path for delivery bundles
	GHCRDeliveryBundlePath = GHCRPackagesPath + "delivery/"
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
	err = op.getBundleManifest()
	if err != nil {
		return nil, err
	}
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
	containsSBOMs := false

	// iterate through Zarf image manifests and find the Zarf pkg's sboms.tar
	for _, layer := range root.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == config.BundleYAML {
			continue
		}
		zarfManifest, err := op.OrasRemote.FetchManifest(layer)
		if err != nil {
			return err
		}
		// grab descriptor for sboms.tar
		sbomDesc := zarfManifest.Locate(config.SBOMsTar)
		if oci.IsEmptyDescriptor(sbomDesc) {
			message.Warnf("%s not found in Zarf pkg", config.SBOMsTar)
			continue
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
		containsSBOMs = true
	}
	if extractSBOM {
		if !containsSBOMs {
			message.Warnf("Cannot extract, no SBOMs found in bundle")
			return nil
		}
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

// LoadBundle loads a bundle's uds-bundle.yaml and Zarf packages from a remote source
func (op *ociProvider) LoadBundle(_ int) (PathMap, error) {
	var layersToPull []ocispec.Descriptor
	estimatedBytes := int64(0)

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

	for _, pkg := range bundle.Packages {
		// grab sha of zarf image manifest and pull it down
		sha := strings.Split(pkg.Ref, "@sha256:")[1] // this is where we use the SHA appended to the Zarf pkg inside the bundle
		manifestDesc := op.manifest.Locate(sha)
		if err != nil {
			return nil, err
		}
		manifestBytes, err := op.FetchLayer(manifestDesc)
		if err != nil {
			return nil, err
		}
		// unmarshal the zarf image manifest and add it to the layers to pull
		var manifest oci.ZarfOCIManifest
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			return nil, err
		}
		layersToPull = append(layersToPull, manifestDesc)
		progressBar := message.NewProgressBar(int64(len(manifest.Layers)), fmt.Sprintf("Verifying layers in Zarf package: %s", pkg.Name))
		// go through the layers in the zarf image manifest and check if they exist in the remote
		for _, layer := range manifest.Layers {
			ok, err := op.Repo().Blobs().Exists(op.ctx, layer)
			progressBar.Add(1)
			estimatedBytes += layer.Size
			if err != nil {
				return nil, err
			}
			// if the layer exists in the remote, add it to the layers to pull
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

	// grab the bundle root manifest and add it to the layers to pull
	rootDesc, err := op.ResolveRoot()
	if err != nil {
		return nil, err
	}
	layersToPull = append(layersToPull, rootDesc)

	// create copy options for oras.Copy()
	copyOpts := utils.CreateCopyOpts(layersToPull, config.CommonOptions.OCIConcurrency)

	// Create a thread to update a progress bar as we save the package to disk
	doneSaving := make(chan int)
	errChan := make(chan int)
	var wg sync.WaitGroup
	wg.Add(1)
	go zarfUtils.RenderProgressBarForLocalDirWrite(op.dst, estimatedBytes, &wg, doneSaving, errChan, fmt.Sprintf("Pulling bundle: %s", bundle.Metadata.Name), fmt.Sprintf("Successfully pulled bundle: %s", bundle.Metadata.Name))
	// note that in this case oras.Copy() copies using the bundle root manifest, not the packages directly
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
	return fmt.Errorf("moving bundles in between remote registries not yet supported")
}

// Returns the validated source path based on the provided oci source path
func getOCIValidatedSource(source string) (string, error) {
	originalSource := source

	platform := ocispec.Platform{
		Architecture: config.GetArch(),
		OS:           oci.MultiOS,
	}
	// Check provided repository path
	sourceWithOCI := EnsureOCIPrefix(source)
	remote, err := oci.NewOrasRemote(sourceWithOCI, platform)
	if err == nil {
		source = sourceWithOCI
		_, err = remote.ResolveRoot()

	}
	// if root didn't resolve, expand the path
	if err != nil {
		// Check in ghcr uds bundle path
		source = GHCRUDSBundlePath + originalSource
		remote, err = oci.NewOrasRemote(source, platform)
		if err == nil {
			_, err = remote.ResolveRoot()
		}
		if err != nil {
			message.Debugf("%s: not found", source)
			// Check in delivery bundle path
			source = GHCRDeliveryBundlePath + originalSource
			remote, err = oci.NewOrasRemote(source, platform)
			if err == nil {
				_, err = remote.ResolveRoot()
			}
			if err != nil {
				message.Debugf("%s: not found", source)
				// Check in packages bundle path
				source = GHCRPackagesPath + originalSource
				remote, err = oci.NewOrasRemote(source, platform)
				if err == nil {
					_, err = remote.ResolveRoot()
				}
				if err != nil {
					message.Fatalf(nil, "%s: not found", originalSource)
				}
			}
		}
	}
	message.Debugf("%s: found", source)
	return source, nil
}

// ValidateArch validates that the passed in arch matches the cluster arch
func ValidateArch(arch string) error {
	// compare bundle arch and cluster arch
	var clusterArchs []string
	c, err := cluster.NewCluster()
	if err != nil {
		return err
	}
	if c != nil {
		clusterArchs, err = c.GetArchitectures()
		if err == nil {
			return err
		}
		// check if bundle arch is in clusterArchs
		if !slices.Contains(clusterArchs, arch) {
			return fmt.Errorf("arch %s does not match cluster arch, %s", arch, clusterArchs)
		}
	}
	return nil
}

// CheckOCISourcePath checks that provided oci source path is valid, and updates it if it's missing the full path
func CheckOCISourcePath(source string) (string, error) {
	validTarballPath := utils.IsValidTarballPath(source)
	var err error
	if !validTarballPath {
		source, err = getOCIValidatedSource(source)
		if err != nil {
			return "", err
		}
	}
	return source, nil
}

// EnsureOCIPrefix ensures oci prefix is part of provided remote source path, and adds it if it's not
func EnsureOCIPrefix(source string) string {
	var ociPrefix = "oci://"
	if source[:len(ociPrefix)] != ociPrefix {
		return ociPrefix + source
	}
	return source
}

// ZarfPackageNameMap returns the uds bundle zarf package name to actual zarf package name mappings from the oci provider
func (op *ociProvider) ZarfPackageNameMap() (map[string]string, error) {
	if err := op.getBundleManifest(); err != nil {
		return nil, err
	}

	loaded, err := op.LoadBundleMetadata()
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

	nameMap := make(map[string]string)
	for _, pkg := range bundle.Packages {
		sha := strings.Split(pkg.Ref, "@sha256:")[1] // this is where we use the SHA appended to the Zarf pkg inside the bundle
		manifestDesc := op.manifest.Locate(sha)
		nameMap[manifestDesc.Annotations[config.UDSPackageNameAnnotation]] = manifestDesc.Annotations[config.ZarfPackageNameAnnotation]
	}
	return nameMap, nil
}
