// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRemoteCacheFallback(t *testing.T) {
	for _, phase := range []string{"initialization", "population"} {
		t.Run(phase, func(t *testing.T) {
			privateKey, publicKey := libraryKeyPair(t)
			packagePath := createLibraryPackageWithSigning(t, t.TempDir(), "cache-fallback", privateKey)
			ref := "oci://" + publishLibraryPackage(t, packagePath, startLibraryRegistry(t))
			root := t.TempDir()
			bundleFile := writePackageVerificationBundle(t, root, ref, publicKey)
			config := libraryRegistryConfig(libraryFixtureConfig(t))
			cacheDir := filepath.Join(t.TempDir(), "cache")
			blockedPath := filepath.Join(cacheDir, "bundle-oci", "images")
			if phase == "population" {
				blockedPath = filepath.Join(blockedPath, "ingest")
			}
			// A file where the cache needs a directory fails even when tests
			// run as root. The registry and package remain fully available.
			require.NoError(t, os.MkdirAll(filepath.Dir(blockedPath), 0o700))
			require.NoError(t, os.WriteFile(blockedPath, []byte("cache obstruction"), 0o600))
			config.Options.CacheDir = cacheDir
			streams, _, out, errOut := iostreams.NewTestIOStreams()

			created, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
				Config:  config,
				Streams: streams,
				Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
			})
			require.NoError(t, err)
			require.NotNil(t, created)
			require.FileExists(t, created.OutputPath)
			assert.Empty(t, out.String())
			assert.Contains(t, errOut.String(), "retrying retrieval once without caching")
			assert.Equal(t, cacheDir, config.Options.CacheDir)
			contents, err := os.ReadFile(blockedPath)
			require.NoError(t, err)
			assert.Equal(t, "cache obstruction", string(contents), "fallback must not repair or remove the shared cache")
			entries, err := os.ReadDir(config.Options.TmpDir)
			require.NoError(t, err)
			assert.Empty(t, entries, "both attempts must clean up their workspaces")

			inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: created.OutputPath, Config: config})
			require.NoError(t, err)
			require.Len(t, inspected.Packages, 1)
			require.NotNil(t, inspected.Packages[0].Signature)
			assert.Equal(t, bundle.PackageVerificationStatusVerified, inspected.Packages[0].Signature.Verification)
			prepared, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, created.OutputPath, t.TempDir(), runtime.GOARCH)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, prepared.Close()) })
			loaded, err := prepared.Loader.LoadPackageLayout(t.Context(), &prepared.Bundle.Packages[0], t.TempDir(), bundle.ZarfPackageLayoutLoadOptions{})
			require.NoError(t, err)
			assert.Equal(t, "cache-fallback", loaded.Layout.PackageDefinition.Metadata.Name)

			// The fallback must not turn off verification: the same broken
			// cache and remote package still reject an unrelated signing key.
			_, wrongKey := libraryKeyPair(t)
			wrongRoot := t.TempDir()
			wrongBundle := writePackageVerificationBundle(t, wrongRoot, ref, wrongKey)
			failed, err := bundle.Create(t.Context(), wrongBundle, bundle.CreateOptions{Config: config, Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned}})
			require.ErrorIs(t, err, bundle.ErrCreateBundle)
			assert.Nil(t, failed)
			assert.NoFileExists(t, filepath.Join(wrongRoot, fmt.Sprintf("uds-bundle-package-verification-%s-1.0.0.tar.zst", runtime.GOARCH)))
		})
	}
}
