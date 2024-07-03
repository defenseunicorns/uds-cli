// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/cache"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils/boci"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/cluster"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	"github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/exp/slices"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	src string
	dst string
	*oci.OrasRemote
	rootManifest *oci.Manifest
}

func (op *ociProvider) getBundleManifest() (*oci.Manifest, error) {
	if op.rootManifest != nil {
		return op.rootManifest, nil
	}
	return nil, fmt.Errorf("bundle root manifest not loaded")
}

// LoadBundleMetadata loads a remote bundle's metadata
func (op *ociProvider) LoadBundleMetadata() (types.PathMap, error) {
	ctx := context.TODO()
	if err := helpers.CreateDirectory(filepath.Join(op.dst, config.BlobsDir), 0700); err != nil {
		return nil, err
	}

	layers, err := op.PullPaths(ctx, filepath.Join(op.dst, config.BlobsDir), config.BundleAlwaysPull)
	if err != nil {
		return nil, err
	}

	loaded := make(types.PathMap)
	for _, layer := range layers {
		rel := layer.Annotations[ocispec.AnnotationTitle]
		abs := filepath.Join(op.dst, config.BlobsDir, rel)
		absSha := filepath.Join(op.dst, config.BlobsDir, layer.Digest.Encoded())
		if err := os.Rename(abs, absSha); err != nil {
			return nil, err
		}
		loaded[rel] = absSha
	}
	return loaded, nil
}

// CreateBundleSBOM creates a bundle-level SBOM from the underlying Zarf packages, if the Zarf package contains an SBOM
func (op *ociProvider) CreateBundleSBOM(extractSBOM bool) error {
	ctx := context.TODO()
	SBOMArtifactPathMap := make(types.PathMap)
	root, err := op.FetchRoot(ctx)
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
		zarfManifest, err := op.OrasRemote.FetchManifest(ctx, layer)
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
		sbomBytes, err := op.OrasRemote.FetchLayer(ctx, sbomDesc)
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

// LoadBundle loads a bundle from a remote source
func (op *ociProvider) LoadBundle(opts types.BundlePullOptions, _ int) (*types.UDSBundle, types.PathMap, error) {
	ctx := context.TODO()
	var bundle types.UDSBundle

	// pull the bundle's metadata + sig
	loaded, err := op.LoadBundleMetadata()
	if err != nil {
		return nil, nil, err
	}
	if err := utils.ReadYAMLStrict(loaded[config.BundleYAML], &bundle); err != nil {
		return nil, nil, err
	}

	// validate the sig (if present) before pulling the whole bundle
	if err := ValidateBundleSignature(loaded[config.BundleYAML], loaded[config.BundleYAMLSignature], opts.PublicKeyPath); err != nil {
		return nil, nil, err
	}

	var layersToPull []ocispec.Descriptor
	// need to keep track of all the bundle layers (pulled and cached)
	var bundleLayers []ocispec.Descriptor

	estimatedBytes := int64(0)

	// get the bundle's root manifest
	rootManifest, err := op.getBundleManifest()
	if err != nil {
		return nil, nil, err
	}

	// grab root manifest config
	layersToPull = append(layersToPull, rootManifest.Config)

	store, err := ocistore.NewWithContext(ctx, op.dst)
	if err != nil {
		return nil, nil, err
	}

	// grab the bundle root manifest and add it to the layers to pull
	bundleRootDesc, err := op.ResolveRoot(ctx)
	if err != nil {
		return nil, nil, err
	}
	layersToPull = append(layersToPull, bundleRootDesc)
	bundleLayers = append(bundleLayers, layersToPull...)

	for _, pkg := range bundle.Packages {
		// go through the pkg's layers and figure out which ones to pull based on the req'd + selected components
		pkgLayers, _, err := boci.FindBundledPkgLayers(ctx, pkg, rootManifest, op.OrasRemote)
		if err != nil {
			return nil, nil, err
		}

		// check if the layer already exists in the cache or the store
		for _, layer := range pkgLayers {
			exists, err := cache.CheckLayerExists(ctx, layer, store, op.dst)
			if err != nil {
				return nil, nil, err
			}
			// if layers don't already exist on disk, add to layersToPull
			if !exists {
				layersToPull = append(layersToPull, layer)
				estimatedBytes += layer.Size
			}
		}
		bundleLayers = append(bundleLayers, pkgLayers...)
	}

	// pull layers that didn't already exist on disk
	if len(layersToPull) > 0 {
		_, err := boci.CopyLayers(layersToPull, estimatedBytes, op.dst, op.Repo(), store, bundle.Metadata.Name)
		if err != nil {
			return nil, nil, err
		}

		err = cache.AddPulledImgLayers(layersToPull, op.dst)
		if err != nil {
			return nil, nil, err
		}
	}

	for _, layer := range bundleLayers {
		sha := layer.Digest.Encoded()
		loaded[sha] = filepath.Join(op.dst, config.BlobsDir, sha)
	}

	return &bundle, loaded, nil
}

func (op *ociProvider) PublishBundle(_ types.UDSBundle, _ *oci.OrasRemote) error {
	// todo: implement moving bundles from one registry to another
	return fmt.Errorf("moving bundles in between remote registries not yet supported")
}

// Returns the validated source path based on the provided oci source path
func getOCIValidatedSource(source string) (string, error) {
	ctx := context.TODO()
	originalSource := source

	platform := ocispec.Platform{
		Architecture: config.GetArch(),
		OS:           oci.MultiOS,
	}
	// Check provided repository path
	sourceWithOCI := boci.EnsureOCIPrefix(source)
	remote, err := zoci.NewRemote(sourceWithOCI, platform)
	if err == nil {
		source = sourceWithOCI
		_, err = remote.ResolveRoot(ctx)
	}
	// if root didn't resolve, expand the path
	if err != nil {
		// Check in ghcr uds bundle path
		source = GHCRUDSBundlePath + originalSource
		remote, err = zoci.NewRemote(source, platform)
		if err == nil {
			_, err = remote.ResolveRoot(ctx)
		}
		if err != nil {
			message.Debugf("%s: not found", source)
			// Check in delivery bundle path
			source = GHCRDeliveryBundlePath + originalSource
			remote, err = zoci.NewRemote(source, platform)
			if err == nil {
				_, err = remote.ResolveRoot(ctx)
			}
			if err != nil {
				message.Debugf("%s: not found", source)
				// Check in packages bundle path
				source = GHCRPackagesPath + originalSource
				remote, err = zoci.NewRemote(source, platform)
				if err == nil {
					_, err = remote.ResolveRoot(ctx)
				}
				if err != nil {
					errMsg := fmt.Sprintf("%s: not found", originalSource)
					message.Debug(errMsg)
					return "", errors.New(errMsg)
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
	clusterArchs := []string{}
	c, err := cluster.NewCluster()
	if err != nil {
		message.Debugf("error creating cluster object: %s", err)
	}

	if c != nil {
		nodeList, err := c.Clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("unable to get cluster architectures")
		}

		for _, node := range nodeList.Items {
			clusterArchs = append(clusterArchs, node.Status.NodeInfo.Architecture)
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
