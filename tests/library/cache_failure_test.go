// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateRemoteCacheFailuresRecoverWithoutCache covers unsigned create with
// a filesystem obstruction. One call must recover without changing its config.
func TestCreateRemoteCacheFailuresRecoverWithoutCache(t *testing.T) {
	for _, phase := range []string{"initialization", "population"} {
		t.Run(phase, func(t *testing.T) {
			packagePath := createLibraryPackage(t, t.TempDir(), "cache-failure")
			ref := "oci://" + publishLibraryPackage(t, packagePath, startLibraryRegistry(t))
			root := t.TempDir()
			bundleFile := filepath.Join(root, libraryBundleFileName)
			require.NoError(t, os.WriteFile(bundleFile, fmt.Appendf(nil, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = "cache-failure"
  version = "1.0.0"
}
package "remote" {
  source = %q
  signature_verification { verify = false }
}
`, ref), 0o600))
			config := libraryRegistryConfig(libraryFixtureConfig(t))
			cacheDir, blockedPath := obstructLibraryCache(t, phase)
			config.Options.CacheDir = cacheDir

			// One create call must recover automatically: the cache remains
			// obstructed, but the uncached retry can retrieve the package.
			result, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
				Config:  config,
				Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.FileExists(t, result.OutputPath)
			assert.Equal(t, cacheDir, config.Options.CacheDir)
			contents, err := os.ReadFile(blockedPath)
			require.NoError(t, err)
			assert.Equal(t, "cache obstruction", string(contents))
			entries, err := os.ReadDir(config.Options.TmpDir)
			require.NoError(t, err)
			assert.Empty(t, entries, "both attempts must clean up their temporary workspaces")
		})
	}
}

// TestPullRemoteCacheFailuresRecoverWithoutCache checks that a single unsigned
// pull recovers by retrying without caching, just as remote package create does.
func TestPullRemoteCacheFailuresRecoverWithoutCache(t *testing.T) {
	for _, phase := range []string{"initialization", "population"} {
		t.Run(phase, func(t *testing.T) {
			fixture := createLibraryBundle(t)
			config := libraryRegistryConfig(fixture.Config)
			ref := startLibraryRegistry(t) + "/bundles/cache-failure:v1"
			_, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
			require.NoError(t, err)
			cacheDir, blockedPath := obstructLibraryCache(t, phase)
			config.Options.CacheDir = cacheDir
			targetDir := t.TempDir()

			// One pull call must handle the cache failure and uncached retry;
			// the caller must not have to change configuration and try again.
			result, err := bundle.Pull(t.Context(), ref, targetDir, bundle.PullOptions{
				Config:                    config,
				SkipSignatureVerification: true,
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.FileExists(t, result.OutputPath)
			assert.Equal(t, cacheDir, config.Options.CacheDir)
			contents, err := os.ReadFile(blockedPath)
			require.NoError(t, err)
			assert.Equal(t, "cache obstruction", string(contents))
			entries, err := os.ReadDir(config.Options.TmpDir)
			require.NoError(t, err)
			assert.Empty(t, entries, "both attempts must clean up their temporary workspaces")
		})
	}
}

// TestRemoteCacheInvalidIndexDoesNotRetry documents the boundary of automatic
// recovery. Upstream index decoding errors do not identify a cache filesystem
// failure, so they propagate. Explicitly disabling caching then demonstrates
// that the registry and source content remain usable without repairing the cache.
func TestRemoteCacheInvalidIndexDoesNotRetry(t *testing.T) {
	registryHost := startLibraryRegistry(t)
	fixture, bundleRef, policy := publishSignedLibraryBundle(t, registryHost)
	privateKey, publicKey := libraryKeyPair(t)
	packagePath := createLibraryPackageWithSigning(t, t.TempDir(), "invalid-cache-index", privateKey)
	packageRef := "oci://" + publishLibraryPackage(t, packagePath, registryHost)
	for _, operation := range []string{"create", "pull", "inspect", "remove preparation"} {
		t.Run(operation, func(t *testing.T) {
			config := libraryRegistryConfig(fixture.Config)
			config.Options.TmpDir = t.TempDir()
			cacheDir := t.TempDir()
			config.Options.CacheDir = cacheDir
			indexPath := filepath.Join(cacheDir, "bundle-oci", "images", "index.json")
			require.NoError(t, os.MkdirAll(filepath.Dir(indexPath), 0o700))
			invalidIndex := []byte("not valid JSON\n")
			require.NoError(t, os.WriteFile(indexPath, invalidIndex, 0o600))
			streams, _, out, errOut := iostreams.NewTestIOStreams()
			outputDir := t.TempDir()
			var bundleFile string
			if operation == "create" {
				bundleFile = writePackageVerificationBundle(t, outputDir, packageRef, publicKey)
			}
			// Use public entrypoints throughout. Remove stops at deliberately
			// invalid selection, after metadata and verification, before any
			// cluster access or removal action.
			attempt := func() error {
				switch operation {
				case "create":
					result, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
						Config: config, Streams: streams, Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
					})
					if err != nil {
						assert.Nil(t, result)
					} else {
						require.NotNil(t, result)
						assert.FileExists(t, result.OutputPath)
					}
					return err
				case "pull":
					result, err := bundle.Pull(t.Context(), bundleRef, outputDir, bundle.PullOptions{Config: config, Verification: policy, Streams: streams})
					if err != nil {
						assert.Nil(t, result)
					} else {
						require.NotNil(t, result)
						assert.FileExists(t, result.OutputPath)
					}
					return err
				case "inspect":
					result, err := bundle.Inspect(t.Context(), bundle.InspectOptions{
						Source: bundleRef, Config: config, Streams: streams, SkipSignatureVerification: true,
					})
					if err != nil {
						assert.Nil(t, result)
					} else {
						require.NotNil(t, result)
						assert.Equal(t, "library-fixture", result.Name)
					}
					return err
				default:
					result, err := bundle.Remove(t.Context(), &bundle.DeploySource{BundlePath: bundleRef}, bundle.RemoveOptions{
						Config: config, Verification: policy, Streams: streams, Packages: []string{"not-in-bundle"}, Force: true,
					})
					assert.Nil(t, result)
					return err
				}
			}
			err := attempt()
			publicError := map[string]error{
				"create": bundle.ErrCreateBundle, "pull": bundle.ErrPullBundle,
				"inspect": bundle.ErrInspectBundle, "remove preparation": bundle.ErrRemoveBundle,
			}[operation]
			require.ErrorIs(t, err, publicError)
			var syntaxErr *json.SyntaxError
			require.ErrorAs(t, err, &syntaxErr)
			assert.NotContains(t, errOut.String(), "retrying", "unclassified index decoding errors must not trigger automatic fallback")
			assert.Equal(t, cacheDir, config.Options.CacheDir)
			artifacts, err := filepath.Glob(filepath.Join(outputDir, "*.tar.zst"))
			require.NoError(t, err)
			assert.Empty(t, artifacts)
			entries, err := os.ReadDir(config.Options.TmpDir)
			require.NoError(t, err)
			assert.Empty(t, entries)

			config.Options.CacheDir = ""
			err = attempt()
			if operation == "remove preparation" {
				require.ErrorIs(t, err, bundle.ErrRemoveBundle)
				require.ErrorContains(t, err, "unknown packages", "metadata and verification must now succeed")
			} else {
				require.NoError(t, err)
			}
			contents, err := os.ReadFile(indexPath)
			require.NoError(t, err)
			assert.Equal(t, invalidIndex, contents)
			assert.Empty(t, out.String())
			entries, err = os.ReadDir(config.Options.TmpDir)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

func obstructLibraryCache(t *testing.T, phase string) (string, string) {
	t.Helper()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	blockedPath := filepath.Join(cacheDir, "bundle-oci", "images")
	if phase == "population" {
		// A file at ingest/ lets the OCI store open normally but prevents
		// writing a downloaded blob's temporary file. Unlike chmod-based
		// tests, this fails deterministically even when run as root.
		blockedPath = filepath.Join(blockedPath, "ingest")
	}
	// A file at images/ prevents opening the OCI store in the first place.
	// All obstructions live in test-owned directories, never the user's cache.
	require.NoError(t, os.MkdirAll(filepath.Dir(blockedPath), 0o700))
	require.NoError(t, os.WriteFile(blockedPath, []byte("cache obstruction"), 0o600))
	return cacheDir, blockedPath
}
