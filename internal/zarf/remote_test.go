// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/errdef"
)

func TestRemoteSourceNewZociRemote_RegistrySchemeNegotiation(t *testing.T) {
	server := httptest.NewTLSServer(registry.New())
	t.Cleanup(server.Close)

	ref := strings.TrimPrefix(server.URL, "https://") + "/test/package:v1"
	source := &remoteSource{
		ref:  ref,
		arch: "amd64",
		opts: ConfigOptions{PlainHTTP: true, SkipTLSVerify: true},
	}

	remote, err := source.newZociRemote(t.Context(), source.ref)
	require.NoError(t, err)
	assert.False(t, remote.Repo().PlainHTTP)
	_, err = remote.Repo().Resolve(t.Context(), "missing")
	require.ErrorIs(t, err, errdef.ErrNotFound, "expected registry response, got: %v", err)
}
