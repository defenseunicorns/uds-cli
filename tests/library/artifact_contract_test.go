// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	fixtureartifact "github.com/defenseunicorns/uds-cli/tests/fixtures/artifact"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const libraryPackageMetadataWithoutSigning = `apiVersion: zarf.dev/v1alpha1
kind: ZarfPackageConfig
metadata:
  name: package-with-unknown-signing-status
  version: 1.0.0
components: []
`

func TestCreateAndInspectPublicContract(t *testing.T) {
	fixture := createLibraryBundle(t)

	require.NotNil(t, fixture.CreateResult)
	assert.Equal(t, "library-fixture", fixture.CreateResult.BundleName)
	assert.Equal(t, filepath.Join(fixture.Root, fmt.Sprintf("uds-bundle-library-fixture-%s-1.0.0.tar.zst", runtime.GOARCH)), fixture.CreateResult.OutputPath)
	require.FileExists(t, fixture.CreateResult.OutputPath)
	entries, err := fixtureartifact.Read(t.Context(), fixture.CreateResult.OutputPath)
	require.NoError(t, err)
	assert.True(t, entries.Paths["oci/oci-layout"])
	assert.True(t, entries.Paths["oci/index.json"])
	assert.NotEmpty(t, entries.BlobPaths())
	hasBundleDefinition, err := entries.HasLayerInArtifact(libraryBundleFileName, fixtureartifact.BundleDefinitionMediaType)
	require.NoError(t, err)
	assert.True(t, hasBundleDefinition)
	hasDefaults, err := entries.HasLayerInArtifact("defaults.uds.hcl", fixtureartifact.BundleDefinitionMediaType)
	require.NoError(t, err)
	assert.True(t, hasDefaults)

	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: fixture.ArtifactPath, Config: fixture.Config})
	require.NoError(t, err)
	assert.Equal(t, "library-fixture", inspected.Name)
	assert.Equal(t, "public library fixture", inspected.Description)
	assert.Equal(t, "1.0.0", inspected.Version)
	assert.NotEmpty(t, inspected.ArtifactDigest)
	assert.Empty(t, inspected.ReconfiguredFrom)
	require.NotNil(t, inspected.BundleSignature)
	assert.Equal(t, bundle.BundleSignatureStatusNotChecked, inspected.BundleSignature.Status)
	require.NotNil(t, inspected.Bundle)
	require.Len(t, inspected.Packages, 2)
	assert.Equal(t, []string{"base", "app"}, []string{inspected.Packages[0].Name, inspected.Packages[1].Name})
	assert.Equal(t, fixture.BaseSource, inspected.Packages[0].Source)
	assert.Equal(t, fixture.AppSource, inspected.Packages[1].Source)
	assert.Equal(t, "base", inspected.Packages[0].Namespace)
	assert.Equal(t, "app", inspected.Packages[1].Namespace)
	assert.Empty(t, inspected.Packages[0].DependsOn)
	assert.Equal(t, []string{"base"}, inspected.Packages[1].DependsOn)
	assert.Equal(t, []string{"values/app.yaml"}, inspected.Packages[1].ValuesFiles)
	assert.Equal(t, []string{"optional"}, inspected.Bundle.Packages[1].OptionalComponents)
	require.NotNil(t, inspected.Packages[0].Signature)
	require.NotNil(t, inspected.Packages[1].Signature)
	assert.Equal(t, bundle.PackageSigningStatusUnsigned, inspected.Packages[0].Signature.Signed)
	assert.Equal(t, bundle.PackageVerificationStatusSkipped, inspected.Packages[0].Signature.Verification)
	assert.Equal(t, bundle.PackageSigningStatusUnsigned, inspected.Packages[1].Signature.Signed)
	assert.Equal(t, bundle.PackageVerificationStatusSkipped, inspected.Packages[1].Signature.Verification)
	assert.Equal(t, []string{"main", "optional"}, libraryPackageComponents(t, fixture, 1))
	assert.Contains(t, libraryPreparedDefaults(t, fixture.ArtifactPath, fixture.Config), "default")
}

func TestPushPullAndInspectPublicContract(t *testing.T) {
	fixture := createLibraryBundle(t)
	config := libraryRegistryConfig(fixture.Config)
	ref := fmt.Sprintf("%s/library/fixture:v1.0.0", startLibraryRegistry(t))
	original, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: fixture.ArtifactPath, Config: fixture.Config})
	require.NoError(t, err)

	pushed, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	require.NotNil(t, pushed)
	assert.Equal(t, ref, pushed.OCIReference)

	targetDir := t.TempDir()
	pulled, err := bundle.Pull(t.Context(), pushed.OCIReference, targetDir, bundle.PullOptions{
		Config:                    config,
		SkipSignatureVerification: true,
	})
	require.NoError(t, err)
	require.NotNil(t, pulled)
	assert.Equal(t, ref, pulled.OCIReference)
	assert.Equal(t, targetDir, filepath.Dir(pulled.OutputPath))
	require.FileExists(t, pulled.OutputPath)

	for _, source := range []string{fixture.ArtifactPath, ref, pulled.OutputPath} {
		inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{
			Source:                    source,
			Config:                    config,
			SkipSignatureVerification: true,
		})
		require.NoError(t, err)
		assert.Equal(t, "library-fixture", inspected.Name)
		assert.Equal(t, "1.0.0", inspected.Version)
		assert.Equal(t, original.ArtifactDigest, inspected.ArtifactDigest)
		assert.Equal(t, []string{"base", "app"}, []string{inspected.Packages[0].Name, inspected.Packages[1].Name})
		assert.Equal(t, bundle.BundleSignatureStatusSkipped, inspected.BundleSignature.Status)
	}
	originalEntries, err := fixtureartifact.Read(t.Context(), fixture.ArtifactPath)
	require.NoError(t, err)
	pulledEntries, err := fixtureartifact.Read(t.Context(), pulled.OutputPath)
	require.NoError(t, err)
	assert.Equal(t, originalEntries.BlobPaths(), pulledEntries.BlobPaths())
	assert.Equal(t, originalEntries.Small["oci/index.json"], pulledEntries.Small["oci/index.json"])

	_, err = bundle.Pull(t.Context(), ref, t.TempDir(), bundle.PullOptions{
		Config:       config,
		Verification: bundle.VerificationPolicy{PublicKey: "unused"},
	})
	require.ErrorIs(t, err, bundle.ErrPullBundle)
	require.ErrorIs(t, err, bundle.ErrBundleNotSigned)
}

func TestInspectStatusConstants(t *testing.T) {
	assert.Equal(t, "verified", bundle.BundleSignatureStatusVerified)
	assert.Equal(t, "not_checked", bundle.BundleSignatureStatusUnverified)
	assert.Equal(t, bundle.BundleSignatureStatusNotChecked, bundle.BundleSignatureStatusUnverified)
	assert.Equal(t, "skipped", bundle.BundleSignatureStatusSkipped)
	assert.Equal(t, "signed", bundle.PackageSigningStatusSigned)
	assert.Equal(t, "unsigned", bundle.PackageSigningStatusUnsigned)
	assert.Equal(t, "unknown", bundle.PackageSigningStatusUnknown)
	assert.Equal(t, "verified", bundle.PackageVerificationStatusVerified)
	assert.Equal(t, "skipped", bundle.PackageVerificationStatusSkipped)
	assert.Equal(t, "unknown", bundle.PackageVerificationStatusUnknown)
}

func TestPullRemoteBundleCacheReuse(t *testing.T) {
	for _, useCache := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%t", useCache), func(t *testing.T) {
			var downloads atomic.Int64
			var blockDownloads atomic.Bool
			handler := registry.New()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") {
					downloads.Add(1)
					if blockDownloads.Load() {
						http.Error(w, "blob downloads disabled for cache reuse test", http.StatusForbidden)
						return
					}
				}
				handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			fixture := createLibraryBundle(t)
			privateKey, publicKey := libraryKeyPair(t)
			require.NoError(t, bundle.Sign(t.Context(), bundle.SignOptions{
				Source:  fixture.ArtifactPath,
				Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
				Config:  fixture.Config,
				TmpDir:  t.TempDir(),
			}))
			config := libraryRegistryConfig(fixture.Config)
			cacheDir := filepath.Join(t.TempDir(), "cache")
			if useCache {
				config.Options.CacheDir = cacheDir
			}
			ref := strings.TrimPrefix(server.URL, "http://") + "/library/cached-pull:v1"
			_, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
			require.NoError(t, err)
			policy := bundle.VerificationPolicy{PublicKey: publicKey}
			pull := func() *bundle.PullResult {
				t.Helper()
				result, err := bundle.Pull(t.Context(), ref, t.TempDir(), bundle.PullOptions{Config: config, Verification: policy})
				require.NoError(t, err)
				require.NotNil(t, result)
				require.FileExists(t, result.OutputPath)
				return result
			}
			cold := pull()
			coldDownloads := downloads.Load()
			require.Positive(t, coldDownloads)
			if useCache {
				require.DirExists(t, filepath.Join(cacheDir, "bundle-oci", "images", "blobs", "sha256"))
				assert.NoDirExists(t, filepath.Join(cacheDir, "images"))
				assert.NoDirExists(t, filepath.Join(cacheDir, "layers"))
				// Leave registry resolution and signature discovery available,
				// but forbid blob downloads. A fresh pull destination cannot
				// supply those bytes: they must come from the persistent cache.
				blockDownloads.Store(true)
			}
			warm := pull()
			assert.NotEqual(t, filepath.Dir(cold.OutputPath), filepath.Dir(warm.OutputPath))
			if useCache {
				assert.Equal(t, coldDownloads, downloads.Load(), "the second pull must reuse cached blobs")
				// Warm bytes do not authorize a different verification key.
				// Since downloads remain blocked, rejection exercises cached
				// signature evidence rather than a fresh network download.
				_, wrongPublicKey := libraryKeyPair(t)
				failedDir := t.TempDir()
				failed, err := bundle.Pull(t.Context(), ref, failedDir, bundle.PullOptions{
					Config:       config,
					Verification: bundle.VerificationPolicy{PublicKey: wrongPublicKey},
				})
				require.ErrorIs(t, err, bundle.ErrPullBundle)
				assert.ErrorContains(t, err, "verifying bundle signature", "failure must be verification, not an unavailable download")
				assert.Nil(t, failed)
				artifacts, err := filepath.Glob(filepath.Join(failedDir, "*.tar.zst"))
				require.NoError(t, err)
				assert.Empty(t, artifacts)
				assert.Equal(t, coldDownloads, downloads.Load())
			} else {
				assert.Greater(t, downloads.Load(), coldDownloads)
				assert.NoDirExists(t, cacheDir)
			}
			original, err := fixtureartifact.Read(t.Context(), fixture.ArtifactPath)
			require.NoError(t, err)
			pulled, err := fixtureartifact.Read(t.Context(), warm.OutputPath)
			require.NoError(t, err)
			assert.Equal(t, original.BlobPaths(), pulled.BlobPaths())
			assert.Equal(t, original.Small["oci/index.json"], pulled.Small["oci/index.json"])
			require.NoError(t, bundle.Verify(t.Context(), bundle.VerifyOptions{Source: warm.OutputPath, Policy: policy, Config: config, TmpDir: t.TempDir()}))
			// Load actual package content from the downloaded artifact through
			// the public API, not from the remote source or shared cache.
			prepared, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, warm.OutputPath, t.TempDir(), runtime.GOARCH)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, prepared.Close()) })
			for i := range prepared.Bundle.Packages {
				pkg := &prepared.Bundle.Packages[i]
				loaded, err := prepared.Loader.LoadPackageLayout(t.Context(), pkg, t.TempDir(), bundle.ZarfPackageLayoutLoadOptions{})
				require.NoError(t, err)
				assert.Equal(t, pkg.Name, loaded.Layout.PackageDefinition.Metadata.Name)
			}
		})
	}
}

func TestInspectUnknownPackageMetadata(t *testing.T) {
	fixture := createLibraryBundle(t)
	unknown := rewriteLibraryArtifact(t, fixture.ArtifactPath, func(root string) {
		rewriteUnknownPackageMetadata(t, root)
	})

	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: unknown, Config: fixture.Config})
	require.NoError(t, err)
	for _, pkg := range inspected.Packages {
		require.NotNil(t, pkg.Signature)
		assert.Equal(t, bundle.PackageSigningStatusUnknown, pkg.Signature.Signed)
		assert.Equal(t, bundle.PackageVerificationStatusUnknown, pkg.Signature.Verification)
	}
}

func TestRemoteBundleMetadataCacheReuse(t *testing.T) {
	for _, operation := range []string{"inspect", "inspect with verification skipped", "remove preparation"} {
		for _, useCache := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cache=%t", operation, useCache), func(t *testing.T) {
				fixture := createLibraryBundle(t)
				entries, err := fixtureartifact.Read(t.Context(), fixture.ArtifactPath)
				require.NoError(t, err)
				manifests, err := entries.PackageManifestDigests()
				require.NoError(t, err)
				payloads := make(map[string]struct{})
				for digest := range manifests {
					var manifest ocispec.Manifest
					require.NoError(t, json.Unmarshal(entries.Small["oci/blobs/sha256/"+strings.TrimPrefix(digest, "sha256:")], &manifest))
					for _, layer := range manifest.Layers {
						if layer.Annotations[ocispec.AnnotationTitle] != "zarf.yaml" {
							payloads[layer.Digest.String()] = struct{}{}
						}
					}
				}
				require.NotEmpty(t, payloads)
				var downloads, payloadDownloads atomic.Int64
				var blockDownloads atomic.Bool
				handler := registry.New()
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") {
						if _, payload := payloads[filepath.Base(r.URL.Path)]; payload {
							payloadDownloads.Add(1)
							http.Error(w, "package payloads must not be read for metadata", http.StatusForbidden)
							return
						}
						downloads.Add(1)
						if blockDownloads.Load() {
							http.Error(w, "metadata blob downloads blocked", http.StatusForbidden)
							return
						}
					}
					handler.ServeHTTP(w, r)
				}))
				t.Cleanup(server.Close)
				// Bundle evidence exercises inspect's preliminary duplicate-
				// signature check as well as the shared metadata reader. No
				// verification policy is requested: verified inspect deliberately
				// retains its full-content verification workflow.
				privateKey, _ := libraryKeyPair(t)
				require.NoError(t, bundle.Sign(t.Context(), bundle.SignOptions{
					Source: fixture.ArtifactPath, Config: fixture.Config, TmpDir: t.TempDir(),
					Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
				}))
				config := libraryRegistryConfig(fixture.Config)
				cacheDir := filepath.Join(t.TempDir(), "cache")
				if useCache {
					config.Options.CacheDir = cacheDir
				}
				ref := strings.TrimPrefix(server.URL, "http://") + "/library/metadata:v1"
				_, err = bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
				require.NoError(t, err)
				read := func() {
					t.Helper()
					if operation == "remove preparation" {
						// Deliberately fail package selection after metadata preparation
						// but before cluster access. This tests the public remove path
						// without removing anything or requiring a Kubernetes cluster.
						result, err := bundle.Remove(t.Context(), &bundle.DeploySource{BundlePath: ref}, bundle.RemoveOptions{
							Config: config, Packages: []string{"not-in-bundle"}, Force: true, SkipSignatureVerification: true,
						})
						require.ErrorIs(t, err, bundle.ErrRemoveBundle)
						require.ErrorContains(t, err, "unknown packages")
						assert.Nil(t, result)
						return
					}
					result, err := bundle.Inspect(t.Context(), bundle.InspectOptions{
						Source: ref, Config: config, SkipSignatureVerification: operation == "inspect with verification skipped",
					})
					require.NoError(t, err)
					assert.Equal(t, "library-fixture", result.Name)
					require.Len(t, result.Packages, 2)
					assert.Equal(t, "base", result.Packages[0].Name)
					assert.Equal(t, "app", result.Packages[1].Name)
				}
				read()
				coldDownloads := downloads.Load()
				require.Positive(t, coldDownloads)
				if useCache {
					blockDownloads.Store(true)
				}
				read()
				assert.Zero(t, payloadDownloads.Load(), "metadata reads must never download package payloads")
				if useCache {
					assert.Equal(t, coldDownloads, downloads.Load(), "a fresh metadata read must reuse cached blobs")
					for digest := range payloads {
						assert.NoFileExists(t, filepath.Join(cacheDir, "bundle-oci", "images", "blobs", "sha256", strings.TrimPrefix(digest, "sha256:")))
					}
				} else {
					assert.Greater(t, downloads.Load(), coldDownloads)
					assert.NoDirExists(t, cacheDir)
				}
			})
		}
	}
}

func libraryRegistryConfig(config *bundle.UDSBundleConfig) *bundle.UDSBundleConfig {
	copy := *config
	options := *config.Options
	options.PlainHTTP = true
	copy.Options = &options
	return &copy
}

func TestCreateRemotePackageCacheReuse(t *testing.T) {
	for _, useCache := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%t", useCache), func(t *testing.T) {
			var downloads atomic.Int64
			var blockDownloads atomic.Bool
			registryHandler := registry.New()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") {
					downloads.Add(1)
					if blockDownloads.Load() {
						http.Error(w, "blob downloads disabled for cache reuse test", http.StatusForbidden)
						return
					}
				}
				registryHandler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			packagePath := createLibraryPackage(t, t.TempDir(), "cached-package")
			ref := "oci://" + publishLibraryPackage(t, packagePath, strings.TrimPrefix(server.URL, "http://"))
			config := libraryRegistryConfig(libraryFixtureConfig(t))
			cacheDir := filepath.Join(t.TempDir(), "cache")
			if useCache {
				config.Options.CacheDir = cacheDir
			}

			create := func() *bundle.CreateResult {
				t.Helper()
				root := t.TempDir()
				bundleFile := filepath.Join(root, libraryBundleFileName)
				require.NoError(t, os.WriteFile(bundleFile, []byte(fmt.Sprintf(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = "cache-reuse"
  version = "1.0.0"
}
package "cached-package" {
  source = %q
  signature_verification { verify = false }
}
`, ref)), 0o600))
				created, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
					Config:  config,
					Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
				})
				require.NoError(t, err)
				require.FileExists(t, created.OutputPath)
				return created
			}

			create()
			coldDownloads := downloads.Load()
			require.Positive(t, coldDownloads)
			if useCache {
				// Next gives Zarf <uds-cache>/bundle-oci as CachePath; Zarf adds
				// its usual images/ subdirectory. This redirects Zarf's OCI cache
				// to one Next-specific location. The sibling images/ and layers/
				// paths are Zarf's default location and Legacy's cache, respectively.
				require.DirExists(t, filepath.Join(cacheDir, "bundle-oci", "images", "blobs", "sha256"))
				assert.NoDirExists(t, filepath.Join(cacheDir, "images"), "Next must not populate Zarf's default cache location")
				assert.NoDirExists(t, filepath.Join(cacheDir, "layers"), "Next must not populate Legacy's layer cache")
				blockDownloads.Store(true)
			}

			warm := create()
			if useCache {
				assert.Equal(t, coldDownloads, downloads.Load(), "the second create must reuse cached blobs")
			} else {
				assert.Greater(t, downloads.Load(), coldDownloads, "an unset library cache directory must leave caching disabled")
				assert.NoDirExists(t, cacheDir)
			}
			prepared, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, warm.OutputPath, t.TempDir(), runtime.GOARCH)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, prepared.Close()) })
			require.Len(t, prepared.Bundle.Packages, 1)
			loaded, err := prepared.Loader.LoadPackageLayout(t.Context(), &prepared.Bundle.Packages[0], t.TempDir(), bundle.ZarfPackageLayoutLoadOptions{})
			require.NoError(t, err)
			assert.Equal(t, "cached-package", loaded.Layout.PackageDefinition.Metadata.Name)
		})
	}
}

func TestCreateLocalPackagesDoesNotOpenRemoteCache(t *testing.T) {
	// The fixture uses local Zarf packages that already contain the content
	// needed to assemble the bundle. With no remote retrieval to cache, create
	// must leave the configured remote cache directory uncreated.
	fixture := createLibraryBundle(t)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	fixture.Config.Options.CacheDir = cacheDir
	_, err := bundle.Create(t.Context(), fixture.BundleFile, bundle.CreateOptions{
		Config:  fixture.Config,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.NoError(t, err)
	assert.NoDirExists(t, cacheDir)
}

func TestCreateAndPullSharePackageCache(t *testing.T) {
	for _, createFirst := range []bool{true, false} {
		name := "pull then create"
		if createFirst {
			name = "create then pull"
		}
		t.Run(name, func(t *testing.T) {
			var blockPackageDownloads atomic.Bool
			var blockedDownloads atomic.Int64
			var downloads atomic.Int64
			packageLayers := map[string]struct{}{}
			handler := registry.New()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") {
					downloads.Add(1)
					// The atomic flag publishes the completed, immutable digest
					// set. The handler does not read it while setup is filling it.
					if blockPackageDownloads.Load() {
						if _, shared := packageLayers[filepath.Base(r.URL.Path)]; shared {
							blockedDownloads.Add(1)
							http.Error(w, "shared package layer downloads blocked", http.StatusForbidden)
							return
						}
					}
				}
				handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			host := strings.TrimPrefix(server.URL, "http://")
			privateKey, publicKey := libraryKeyPair(t)
			packagePath := createLibraryPackageWithSigning(t, t.TempDir(), "shared-cache", privateKey)
			packageRef := "oci://" + publishLibraryPackage(t, packagePath, host)
			config := libraryRegistryConfig(libraryFixtureConfig(t))
			config.Options.CacheDir = filepath.Join(t.TempDir(), "cache")
			create := func(cfg *bundle.UDSBundleConfig) (*bundle.CreateResult, error) {
				t.Helper()
				// Each create gets a fresh workspace. The original artifact
				// and its package staging cannot satisfy a later create.
				bundleFile := writePackageVerificationBundle(t, t.TempDir(), packageRef, publicKey)
				return bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
					Config:  cfg,
					Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
				})
			}
			bootstrap := *config
			bootstrapOptions := *config.Options
			bootstrap.Options = &bootstrapOptions
			if !createFirst {
				// Build the fixture without warming the shared cache. Only
				// the upcoming bundle pull may populate that cache.
				bootstrap.Options.CacheDir = ""
			}
			created, err := create(&bootstrap)
			require.NoError(t, err)
			if !createFirst {
				require.NoDirExists(t, config.Options.CacheDir)
			}
			entries, err := fixtureartifact.Read(t.Context(), created.OutputPath)
			require.NoError(t, err)
			manifests, err := entries.PackageManifestDigests()
			require.NoError(t, err)
			require.Len(t, manifests, 1)
			for digest := range manifests {
				var manifest ocispec.Manifest
				require.NoError(t, json.Unmarshal(entries.Small["oci/blobs/sha256/"+strings.TrimPrefix(digest, "sha256:")], &manifest))
				for _, layer := range manifest.Layers {
					packageLayers[layer.Digest.String()] = struct{}{}
				}
			}
			require.NotEmpty(t, packageLayers)
			bundleRef := host + "/bundles/shared-cache:v1"
			_, err = bundle.Push(t.Context(), created.OutputPath, bundleRef, bundle.PushOptions{Config: config})
			require.NoError(t, err)
			pull := func(cfg *bundle.UDSBundleConfig) (*bundle.PullResult, error) {
				t.Helper()
				return bundle.Pull(t.Context(), bundleRef, t.TempDir(), bundle.PullOptions{Config: cfg, SkipSignatureVerification: true})
			}
			if !createFirst {
				beforePull := downloads.Load()
				_, err := pull(config)
				require.NoError(t, err)
				require.Greater(t, downloads.Load(), beforePull)
			}
			for digest := range packageLayers {
				require.FileExists(t, filepath.Join(config.Options.CacheDir, "bundle-oci", "images", "blobs", "sha256", strings.TrimPrefix(digest, "sha256:")))
			}
			// Bundle definitions are not fetched during package retrieval.
			// Block only the shared package layers, leaving bundle-specific
			// content, tag resolution, and registry discovery available.
			blockPackageDownloads.Store(true)
			var reusedArtifact string
			if createFirst {
				pulled, err := pull(config)
				require.NoError(t, err)
				reusedArtifact = pulled.OutputPath
			} else {
				warm, err := create(config)
				require.NoError(t, err)
				reusedArtifact = warm.OutputPath
			}
			assert.Zero(t, blockedDownloads.Load(), "the second operation must reuse package layers without downloading them")
			inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: reusedArtifact, Config: config})
			require.NoError(t, err)
			require.Len(t, inspected.Packages, 1)
			require.NotNil(t, inspected.Packages[0].Signature)
			assert.Equal(t, bundle.PackageVerificationStatusVerified, inspected.Packages[0].Signature.Verification)
			reusedEntries, err := fixtureartifact.Read(t.Context(), reusedArtifact)
			require.NoError(t, err)
			assert.Equal(t, entries.BlobPaths(), reusedEntries.BlobPaths())
			assert.Equal(t, entries.Small["oci/index.json"], reusedEntries.Small["oci/index.json"])

			// Control: with a different, empty cache the exact same operation
			// must fail. This proves the download blocker is effective and the
			// successful case was not satisfied by some unrelated local store.
			coldConfig := *config
			coldOptions := *config.Options
			coldOptions.CacheDir = t.TempDir()
			coldConfig.Options = &coldOptions
			if createFirst {
				failed, err := pull(&coldConfig)
				require.ErrorIs(t, err, bundle.ErrPullBundle)
				assert.Nil(t, failed)
			} else {
				failed, err := create(&coldConfig)
				require.ErrorIs(t, err, bundle.ErrCreateBundle)
				assert.Nil(t, failed)
			}
			assert.Positive(t, blockedDownloads.Load(), "an empty cache must attempt a blocked package download")
		})
	}
}

func TestCreateOptionalComponentCanBeExcluded(t *testing.T) {
	fixture := createLibraryBundleWithoutOptionalComponent(t)
	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: fixture.ArtifactPath, Config: fixture.Config})
	require.NoError(t, err)
	assert.Empty(t, inspected.Bundle.Packages[1].OptionalComponents)
	assert.Equal(t, []string{"main"}, libraryPackageComponents(t, fixture, 1))
}

func TestCreateInspectsSignedVerifiedPackage(t *testing.T) {
	privateKey, publicKey := libraryKeyPair(t)
	root := t.TempDir()
	source := createLibraryPackageWithSigning(t, root, "signed", privateKey)
	bundleFile := writePackageVerificationBundle(t, root, source, publicKey)
	config := libraryFixtureConfig(t)

	created, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
		Config:  config,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.NoError(t, err)
	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: created.OutputPath, Config: config})
	require.NoError(t, err)
	require.NotNil(t, inspected.BundleSignature)
	require.Len(t, inspected.Packages, 1)
	require.NotNil(t, inspected.Packages[0].Signature)
	assert.Equal(t, bundle.BundleSignatureStatusNotChecked, inspected.BundleSignature.Status)
	assert.Equal(t, bundle.PackageSigningStatusSigned, inspected.Packages[0].Signature.Signed)
	assert.Equal(t, bundle.PackageVerificationStatusVerified, inspected.Packages[0].Signature.Verification)
}

func TestCreateVerifiesSignedPackageFromOCI(t *testing.T) {
	privateKey, publicKey := libraryKeyPair(t)
	packageRoot := t.TempDir()
	packagePath := createLibraryPackageWithSigning(t, packageRoot, "signed-oci", privateKey)
	ref := "oci://" + publishLibraryPackage(t, packagePath, startLibraryRegistry(t))
	config := libraryRegistryConfig(libraryFixtureConfig(t))
	config.Options.CacheDir = filepath.Join(t.TempDir(), "cache")

	root := t.TempDir()
	bundleFile := writePackageVerificationBundle(t, root, ref, publicKey)
	created, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
		Config:  config,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "package-verification", created.BundleName)
	require.FileExists(t, created.OutputPath)
	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: created.OutputPath, Config: config})
	require.NoError(t, err)
	require.Len(t, inspected.Packages, 1)
	require.NotNil(t, inspected.Packages[0].Signature)
	assert.Equal(t, bundle.PackageSigningStatusSigned, inspected.Packages[0].Signature.Signed)
	assert.Equal(t, bundle.PackageVerificationStatusVerified, inspected.Packages[0].Signature.Verification)

	_, wrongPublicKey := libraryKeyPair(t)
	wrongRoot := t.TempDir()
	wrongBundle := writePackageVerificationBundle(t, wrongRoot, ref, wrongPublicKey)
	failed, err := bundle.Create(t.Context(), wrongBundle, bundle.CreateOptions{
		Config:  config,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.ErrorIs(t, err, bundle.ErrCreateBundle)
	assert.Nil(t, failed)
	artifacts, err := filepath.Glob(filepath.Join(wrongRoot, "*.tar.zst"))
	require.NoError(t, err)
	assert.Empty(t, artifacts)
}

func TestCreatePublicContractSupportsMultipleArchitectures(t *testing.T) {
	for _, architecture := range []string{"amd64", "arm64"} {
		t.Run(architecture, func(t *testing.T) {
			root := t.TempDir()
			source := createLibraryPackageForArchitecture(t, root, "multiarch", architecture)
			bundleFile := filepath.Join(root, libraryBundleFileName)
			require.NoError(t, os.WriteFile(bundleFile, []byte(fmt.Sprintf(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = "multiarch"
  version = "1.0.0"
}
package "multiarch" {
  source = %q
  signature_verification { verify = false }
}
`, source)), 0o600))
			config := libraryFixtureConfigForArchitecture(t, architecture)

			created, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
				Config:  config,
				Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
			})
			require.NoError(t, err)
			require.NotNil(t, created)
			assert.Equal(t, filepath.Join(root, fmt.Sprintf("uds-bundle-multiarch-%s-1.0.0.tar.zst", architecture)), created.OutputPath)
			require.FileExists(t, created.OutputPath)
			assert.Equal(t, architecture, libraryPackageArchitecture(t, created.OutputPath, architecture))
		})
	}
}

func TestCreateRejectsWrongPackageVerificationKey(t *testing.T) {
	privateKey, _ := libraryKeyPair(t)
	_, wrongPublicKey := libraryKeyPair(t)
	root := t.TempDir()
	source := createLibraryPackageWithSigning(t, root, "signed", privateKey)
	bundleFile := writePackageVerificationBundle(t, root, source, wrongPublicKey)

	created, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
		Config:  libraryFixtureConfig(t),
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.ErrorIs(t, err, bundle.ErrCreateBundle)
	assert.Nil(t, created)
	assert.NoFileExists(t, filepath.Join(root, fmt.Sprintf("uds-bundle-package-verification-%s-1.0.0.tar.zst", runtime.GOARCH)))
}

func libraryPackageComponents(t *testing.T, fixture libraryBundleFixture, packageIndex int) []string {
	t.Helper()
	source, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, fixture.ArtifactPath, t.TempDir(), runtime.GOARCH)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, source.Close()) })
	require.NotNil(t, source.Loader)
	require.Len(t, source.Bundle.Packages, packageIndex+1)

	loaded, err := source.Loader.LoadPackageLayout(t.Context(), &source.Bundle.Packages[packageIndex], t.TempDir(), bundle.ZarfPackageLayoutLoadOptions{})
	require.NoError(t, err)
	definition := loaded.Layout.PackageDefinition
	assert.Equal(t, runtime.GOARCH, definition.Metadata.Architecture)
	components := make([]string, len(definition.Components))
	for i, component := range definition.Components {
		components[i] = component.Name
	}
	return components
}

func libraryPackageArchitecture(t *testing.T, artifactPath, architecture string) string {
	t.Helper()
	source, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, artifactPath, t.TempDir(), architecture)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, source.Close()) })
	require.NotNil(t, source.Loader)
	require.Len(t, source.Bundle.Packages, 1)

	loaded, err := source.Loader.LoadPackageLayout(t.Context(), &source.Bundle.Packages[0], t.TempDir(), bundle.ZarfPackageLayoutLoadOptions{})
	require.NoError(t, err)
	return loaded.Layout.PackageDefinition.Metadata.Architecture
}

func rewriteUnknownPackageMetadata(t *testing.T, root string) {
	t.Helper()
	indexPath := filepath.Join(root, "oci", "index.json")
	indexBytes, err := os.ReadFile(indexPath)
	require.NoError(t, err)
	var index ocispec.Index
	require.NoError(t, json.Unmarshal(indexBytes, &index))

	var hclRewritten, packageMetadataRewritten bool
	for i := range index.Manifests {
		manifestPath := filepath.Join(root, "oci", "blobs", "sha256", index.Manifests[i].Digest.Hex())
		manifestBytes, err := os.ReadFile(manifestPath)
		require.NoError(t, err)
		var manifest ocispec.Manifest
		require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
		changed := false
		for j := range manifest.Layers {
			title := manifest.Layers[j].Annotations[ocispec.AnnotationTitle]
			if title != libraryBundleFileName && title != "zarf.yaml" {
				continue
			}
			var contents []byte
			switch title {
			case libraryBundleFileName:
				layerPath := filepath.Join(root, "oci", "blobs", "sha256", manifest.Layers[j].Digest.Hex())
				contents, err = os.ReadFile(layerPath)
				require.NoError(t, err)
				contents = removeLibraryPackageVerification(t, contents)
				hclRewritten = true
			case "zarf.yaml":
				contents = []byte(libraryPackageMetadataWithoutSigning)
				packageMetadataRewritten = true
			}
			writeLibraryBlob(t, root, &manifest.Layers[j], contents)
			changed = true
		}
		if changed {
			updated, err := json.Marshal(manifest)
			require.NoError(t, err)
			writeLibraryBlob(t, root, &index.Manifests[i], updated)
		}
	}
	require.True(t, hclRewritten)
	require.True(t, packageMetadataRewritten)
	updatedIndex, err := json.Marshal(index)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(indexPath, updatedIndex, 0o600))
}

func removeLibraryPackageVerification(t *testing.T, contents []byte) []byte {
	t.Helper()
	file, diagnostics := hclwrite.ParseConfig(contents, libraryBundleFileName, hcl.InitialPos)
	require.False(t, diagnostics.HasErrors())
	removed := false
	for _, block := range file.Body().Blocks() {
		if block.Type() != "package" {
			continue
		}
		for _, child := range block.Body().Blocks() {
			if child.Type() == "signature_verification" {
				removed = block.Body().RemoveBlock(child) || removed
			}
		}
	}
	require.True(t, removed)
	return file.Bytes()
}

func writeLibraryBlob(t *testing.T, root string, descriptor *ocispec.Descriptor, contents []byte) {
	t.Helper()
	digest := godigest.FromBytes(contents)
	blobsRoot, err := os.OpenRoot(filepath.Join(root, "oci", "blobs", "sha256"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, blobsRoot.Close()) })
	require.NoError(t, blobsRoot.WriteFile(digest.Hex(), contents, 0o600))
	descriptor.Digest = digest
	descriptor.Size = int64(len(contents))
}

func writePackageVerificationBundle(t *testing.T, root, source, publicKey string) string {
	t.Helper()
	bundleFile := filepath.Join(root, libraryBundleFileName)
	require.NoError(t, os.WriteFile(bundleFile, []byte(fmt.Sprintf(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = "package-verification"
  version = "1.0.0"
}
package "signed" {
  source = %q
  signature_verification { public_key = %q }
}
`, source, publicKey)), 0o600))
	return bundleFile
}
