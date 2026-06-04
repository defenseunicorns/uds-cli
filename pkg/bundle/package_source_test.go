// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPackageSource_Remote(t *testing.T) {
	opts := ConfigOptions{
		Architecture:  "amd64",
		PlainHTTP:     true,
		SkipTLSVerify: true,
		Concurrency:   4,
	}

	tests := []struct {
		name    string
		source  string
		wantRef string
	}{
		{
			name:    "with oci:// scheme",
			source:  "oci://ghcr.io/org/repo:v1.0.0",
			wantRef: "ghcr.io/org/repo:v1.0.0",
		},
		{
			name:    "without scheme",
			source:  "ghcr.io/org/repo:v1.0.0",
			wantRef: "ghcr.io/org/repo:v1.0.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := NewPackageSource(tt.source, opts, "/bundle/dir", iostreams.IOStreams{})
			remote, ok := src.(*remoteSource)
			require.True(t, ok, "expected *remoteSource")
			assert.Equal(t, tt.wantRef, remote.ref)
			assert.Equal(t, "amd64", remote.arch)
			assert.Equal(t, opts, remote.opts)
		})
	}
}

func TestNewPackageSource_Local(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "relative path", source: "./my-package"},
		{name: "tar.zst file", source: "my-package.tar.zst"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ConfigOptions{Architecture: "arm64", TmpDir: "/custom/tmp"}
			src := NewPackageSource(tt.source, opts, "/bundle/dir", iostreams.IOStreams{})
			local, ok := src.(*localSource)
			require.True(t, ok, "expected *localSource")
			assert.Equal(t, tt.source, local.path)
			assert.Equal(t, "arm64", local.arch)
			assert.Equal(t, "/bundle/dir", local.bundleDir)
			assert.Equal(t, "/custom/tmp", local.tmpDir)
		})
	}
}
