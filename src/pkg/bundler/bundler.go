// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundler defines behavior for bundling packages
package bundler

import (
	"github.com/defenseunicorns/uds-cli/src/types"
)

type Bundler struct {
	bundle    *types.UDSBundle
	output    string
	tmpDstDir string
}

type Pusher interface {
}

type Options struct {
	Bundle    *types.UDSBundle
	Output    string
	TmpDstDir string
}

func NewBundler(opts *Options) *Bundler {
	b := Bundler{
		bundle:    opts.Bundle,
		output:    opts.Output,
		tmpDstDir: opts.TmpDstDir,
	}
	return &b
}

func (b *Bundler) Create() error {
	if b.output == "" {
		localBundle := NewLocalBundle(&LocalBundleOpts{Bundle: b.bundle, TmpDstDir: b.tmpDstDir})
		err := localBundle.create(nil)
		if err != nil {
			return err
		}
	} else {
		remoteBundle := NewRemoteBundle(&RemoteBundleOpts{Bundle: b.bundle, Output: b.output})
		err := remoteBundle.create(nil)
		if err != nil {
			return err
		}
	}
	return nil
}
