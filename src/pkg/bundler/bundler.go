// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundler defines behavior for bundling packages
package bundler

import (
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
)

// Bundler is used for bundling packages
type Bundler struct {
	bundle    *types.UDSBundle
	output    string
	tmpDstDir string
	sourceDir string
}

// Pusher is the interface for pushing bundles
type Pusher interface {
}

// Options are the options for creating a bundler
type Options struct {
	Bundle    *types.UDSBundle
	Output    string
	TmpDstDir string
	SourceDir string
}

// NewBundler creates a new bundler
func NewBundler(opts *Options) *Bundler {
	b := Bundler{
		bundle:    opts.Bundle,
		output:    opts.Output,
		tmpDstDir: opts.TmpDstDir,
		sourceDir: opts.SourceDir,
	}
	return &b
}

// Create creates a bundle
func (b *Bundler) Create() error {
	if utils.IsRegistryURL(b.output) {
		remoteBundle := NewRemoteBundle(&RemoteBundleOpts{Bundle: b.bundle, Output: b.output})
		err := remoteBundle.create(nil)
		if err != nil {
			return err
		}
	} else {
		localBundle := NewLocalBundle(&LocalBundleOpts{Bundle: b.bundle, TmpDstDir: b.tmpDstDir, SourceDir: b.sourceDir, OutputDir: b.output})
		err := localBundle.create(nil)
		if err != nil {
			return err
		}
	}
	return nil
}
