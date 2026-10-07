// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package pusher

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	zarfoci "github.com/zarf-dev/zarf/src/pkg/oci"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
	"oras.land/oras-go/v2/content"
)

func TestRemoteToRemoteCopiesSelectedLayersAndConfig(t *testing.T) {
	srcServer := httptest.NewServer(registry.New())
	t.Cleanup(srcServer.Close)
	dstServer := httptest.NewServer(registry.New())
	t.Cleanup(dstServer.Close)
	platform := ocispec.Platform{Architecture: "amd64", OS: zarfoci.MultiOS}
	options := zoci.RemoteClientOptions{RemoteOptions: zarfTypes.RemoteOptions{PlainHTTP: true}}
	src, err := zoci.NewRemoteWithOptions(t.Context(), strings.TrimPrefix(srcServer.URL, "http://")+"/test/package:v1", platform, options)
	require.NoError(t, err)
	dst, err := zoci.NewRemoteWithOptions(t.Context(), strings.TrimPrefix(dstServer.URL, "http://")+"/test/package:v1", platform, options)
	require.NoError(t, err)
	push := func(data string) ocispec.Descriptor {
		t.Helper()
		desc := content.NewDescriptorFromBytes("application/vnd.zarf.layer.v1.blob", []byte(data))
		require.NoError(t, src.Repo().Push(t.Context(), desc, bytes.NewReader([]byte(data))))
		return desc
	}
	included := push("included")
	excluded := push("excluded")
	config := push("config")
	p := RemotePusher{cfg: Config{
		PkgRootManifest: &zarfoci.Manifest{Manifest: ocispec.Manifest{Config: config}},
		RemoteSrc:       *src,
		RemoteDst:       *dst,
	}}
	require.NoError(t, p.remoteToRemote([]ocispec.Descriptor{included}))

	exists, err := dst.Repo().Exists(t.Context(), included)
	require.NoError(t, err)
	require.True(t, exists)
	exists, err = dst.Repo().Exists(t.Context(), config)
	require.NoError(t, err)
	require.True(t, exists)
	exists, err = dst.Repo().Exists(t.Context(), excluded)
	require.NoError(t, err)
	require.False(t, exists)
}
