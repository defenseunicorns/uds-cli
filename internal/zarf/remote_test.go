// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	packageoci "github.com/defenseunicorns/pkg/oci"
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
	"oras.land/oras-go/v2/errdef"
)

func TestRemoteSourceVerifyAndIngestFilteredRegistryPackage(t *testing.T) {
	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)

	pkgDir := t.TempDir()
	writeFilteredPackageFiles(t, pkgDir)
	pkgLayout, err := layout.LoadFromDir(t.Context(), pkgDir, layout.PackageLayoutOptions{VerificationStrategy: layout.VerifyNever})
	require.NoError(t, err)
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()

	ref := strings.TrimPrefix(server.URL, "http://") + "/test/filtered:1.0.0"
	remote, err := zoci.NewRemoteWithOptions(t.Context(), ref, ocispec.Platform{Architecture: "amd64", OS: packageoci.MultiOS}, zoci.RemoteClientOptions{RemoteOptions: zarfTypes.RemoteOptions{PlainHTTP: true}})
	require.NoError(t, err)
	_, err = remote.PushPackage(t.Context(), pkgLayout, zoci.PublishOptions{Retries: 1, OCIConcurrency: 1})
	require.NoError(t, err)

	store, err := udsoci.CreateStore(t.TempDir())
	require.NoError(t, err)
	source := &remoteSource{
		ref:  ref,
		arch: "amd64",
		opts: bundleinternal.ConfigOptions{PlainHTTP: true, TmpDir: t.TempDir(), Concurrency: 1},
	}
	rootDesc, err := remote.ResolveRoot(t.Context())
	require.NoError(t, err)
	resolved, err := source.resolveFilteredLayers(t.Context(), filters.Combine(filters.ForDeploy("included", false)))
	require.NoError(t, err)
	assert.Equal(t, rootDesc.Digest.String(), resolved.remote.Repo().Reference.Reference)
	assert.Equal(t, strings.TrimSuffix(ref, ":1.0.0")+"@"+rootDesc.Digest.String(), resolved.remote.Repo().Reference.String())
	loaded, err := source.LoadPackageSpec(t.Context(), filters.Combine(filters.ForDeploy("included", false)))
	require.NoError(t, err)
	assert.Equal(t, rootDesc.Digest.String(), loaded.Digest)
	assert.Equal(t, []string{"included"}, loaded.Components)
	loader := NewSourcePackageLayoutLoader(bundleinternal.ConfigOptions{Architecture: "amd64", PlainHTTP: true, TmpDir: t.TempDir(), Concurrency: 1}, t.TempDir())
	pkg := &spec.Package{Name: "pkg", Source: "oci://" + ref, OptionalComponents: []string{"included"}}
	intended, err := loader.LoadPackageSpec(t.Context(), pkg)
	require.NoError(t, err)
	assert.Equal(t, rootDesc, loader.resolvedRoots[pkg.Name])
	deploymentLayout, err := loader.LoadPackageLayout(t.Context(), pkg, t.TempDir(), LoadOptions{IsPartial: true})
	require.NoError(t, err)
	assert.Equal(t, intended.Digest, deploymentLayout.Layout.Digest())
	require.NoError(t, deploymentLayout.Layout.Cleanup())

	descs, err := source.VerifyAndIngestFiltered(t.Context(), t.TempDir(), layout.PackageLayoutOptions{
		Filter:               filters.Combine(filters.ForDeploy("included", false)),
		VerificationStrategy: layout.VerifyNever,
	}, store)
	require.NoError(t, err)
	require.Len(t, descs, 1)

	manifestBytes, err := udsoci.FetchBytes(t.Context(), store, descs[0])
	require.NoError(t, err)
	var manifest ocispec.Manifest
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
	titles := layerTitles(manifest.Layers)
	assert.Contains(t, titles, layout.ZarfYAML)
	assert.Contains(t, titles, filepath.ToSlash(filepath.Join(layout.ComponentsDir, "included.tar")))
	assert.NotContains(t, titles, filepath.ToSlash(filepath.Join(layout.ComponentsDir, "excluded.tar")))
}

func TestRemoteSourceNewZociRemote_RegistrySchemeNegotiation(t *testing.T) {
	server := httptest.NewTLSServer(registry.New())
	t.Cleanup(server.Close)

	ref := strings.TrimPrefix(server.URL, "https://") + "/test/package:v1"
	source := &remoteSource{
		ref:  ref,
		arch: "amd64",
		opts: bundleinternal.ConfigOptions{PlainHTTP: true, SkipTLSVerify: true},
	}

	remote, err := source.newZociRemote(t.Context())
	require.NoError(t, err)
	assert.False(t, remote.Repo().PlainHTTP)
	_, err = remote.Repo().Resolve(t.Context(), "missing")
	require.ErrorIs(t, err, errdef.ErrNotFound, "expected registry response, got: %v", err)
}

func TestRemoteSourceResolvedReference(t *testing.T) {
	root := ocispec.Descriptor{Digest: digest.FromString("resolved")}
	tests := []struct {
		name string
		ref  string
	}{
		{name: "tag", ref: "registry.example.com/team/package:latest"},
		{name: "digest", ref: "registry.example.com/team/package@" + root.Digest.String()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (&remoteSource{ref: tt.ref, resolvedRoot: &root}).resolvedReference()
			require.NoError(t, err)
			assert.Equal(t, "registry.example.com/team/package@"+root.Digest.String(), got)
		})
	}
}
