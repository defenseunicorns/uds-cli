// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package fetcher

import (
	"context"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry/remote/auth"
)

func TestNewZarfOCIRemotePreservesUDSUserAgent(t *testing.T) {
	originalVersion := config.CLIVersion
	originalInsecure := config.CommonOptions.Insecure
	t.Cleanup(func() {
		config.CLIVersion = originalVersion
		config.CommonOptions.Insecure = originalInsecure
	})
	config.CLIVersion = "test-version"
	config.CommonOptions.Insecure = false

	remote, err := NewZarfOCIRemote(context.Background(), "oci://example.com/repo:tag", ocispec.Platform{OS: "linux", Architecture: "amd64"})
	require.NoError(t, err)

	client, ok := remote.Repo().Client.(*auth.Client)
	require.True(t, ok)
	assert.Equal(t, "uds-cli/test-version", client.Header.Get("User-Agent"))
}
