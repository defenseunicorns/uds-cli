// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundler contains functions for interacting with, managing and deploying UDS packages
package bundler

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/zarf/src/pkg/oci"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
)

// Provider is an interface for processing bundles
//
// operations that are common no matter the source should be implemented on bundler
type Provider interface {
	// LoadBundleMetadata loads a bundle's metadata and signature into the temporary directory and returns a map of the bundle's metadata files
	//
	// these two files are placed in the `dst` directory
	//
	// : if tarball
	// : : extracts the metadata from the tarball
	//
	// : if OCI ref
	// : : pulls the metadata from the OCI ref
	LoadBundleMetadata() (PathMap, error)

	// LoadPackage loads a package with a given `sha` from the bundle into the `destinationDir`
	//
	// : if tarball
	// : : extracts the package from the tarball
	//
	// : if OCI ref
	// : : pulls the package from the OCI ref
	LoadPackage(sha, destinationDir string, concurrency int) (PathMap, error)

	// LoadBundle loads a bundle into the temporary directory and returns a map of the bundle's files
	//
	// (currently only the remote provider utilizes the concurrency parameter)
	LoadBundle(concurrency int) (PathMap, error)

	getBundleManifest() error
}

// PathMap is a map of either absolute paths to relative paths or relative paths to absolute paths
type PathMap map[string]string

// NewProvider returns a new bundler Provider based on the source type
func NewProvider(ctx context.Context, source, destination string) (Provider, error) {
	if utils.IsOCIURL(source) {
		provider := ociProvider{ctx: ctx, src: source, dst: destination}
		remote, err := oci.NewOrasRemote(source)
		if err != nil {
			return nil, err
		}
		provider.OrasRemote = remote
		return &provider, nil
	}
	if !IsValidTarballPath(source) {
		return nil, fmt.Errorf("invalid tarball path: %s", source)
	}
	return &tarballProvider{ctx: ctx, src: source, dst: destination}, nil
}
