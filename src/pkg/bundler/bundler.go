// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundler defines behavior for bundling packages
package bundler

import (
	"errors"
	"fmt"
	"strings"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/utils/helpers"
	"oras.land/oras-go/v2/registry"
)

type Bundler struct {
	bundle     *types.UDSBundle
	createOpts types.BundleCreateOptions
	tmpDstDir  string
}

type Pusher interface {
}

type Options struct {
	// todo comment all these options
	Bundle     *types.UDSBundle
	CreateOpts types.BundleCreateOptions
	TmpDstDir  string
}

func NewBundler(opts *Options) *Bundler {
	b := Bundler{
		bundle:     opts.Bundle,
		createOpts: opts.CreateOpts,
		tmpDstDir:  opts.TmpDstDir,
	}
	return &b
}

func (b *Bundler) Create() error {
	if b.createOpts.Output == "" {
		err := b.createLocalBundle(nil)
		if err != nil {
			return err
		}
	} else {
		err := b.createRemoteBundle(nil)
		if err != nil {
			return err
		}
	}
	return nil
}

// copied from: https://github.com/defenseunicorns/zarf/blob/main/src/pkg/oci/utils.go
func referenceFromMetadata(registryLocation string, metadata *types.UDSMetadata) (string, error) {
	ver := metadata.Version
	if len(ver) == 0 {
		return "", errors.New("version is required for publishing")
	}

	if !strings.HasSuffix(registryLocation, "/") {
		registryLocation = registryLocation + "/"
	}
	registryLocation = strings.TrimPrefix(registryLocation, helpers.OCIURLPrefix)
	raw := fmt.Sprintf("%s%s:%s", registryLocation, metadata.Name, ver)

	message.Debug("Raw OCI reference from metadata:", raw)
	ref, err := registry.ParseReference(raw)
	if err != nil {
		return "", err
	}

	return ref.String(), nil
}
