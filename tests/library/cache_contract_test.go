// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	fixtureartifact "github.com/defenseunicorns/uds-cli/tests/fixtures/artifact"
	"github.com/google/go-containerregistry/pkg/registry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/assemble"
	"github.com/zarf-dev/zarf/src/pkg/packager/load"
	"golang.org/x/sync/errgroup"
)

func TestCacheCreatePullReuse(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, createFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("enabled=%t/create-first=%t", enabled, createFirst), func(t *testing.T) {
				remote := startCacheContractRegistry(t)
				privateKey, publicKey := libraryKeyPair(t)
				pkg := createLibraryPackageWithSigning(t, t.TempDir(), "shared-cache", privateKey)
				packageRef := "oci://" + publishLibraryPackage(t, pkg, remote.host)
				config := libraryRegistryConfig(libraryFixtureConfig(t))
				create := func() string {
					t.Helper()
					// Each call gets a new output and package staging workspace.
					file := writePackageVerificationBundle(t, t.TempDir(), packageRef, publicKey)
					result, err := bundle.Create(t.Context(), file, bundle.CreateOptions{
						Config: config, Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
					})
					require.NoError(t, err)
					require.FileExists(t, result.OutputPath)
					return result.OutputPath
				}
				// Bootstrap without caching, so pushing the fixture cannot warm it.
				artifact := create()
				layers := cacheContractPackageLayers(t, artifact)
				ref := remote.host + "/library/shared-cache:v1"
				_, err := bundle.Push(t.Context(), artifact, ref, bundle.PushOptions{Config: config})
				require.NoError(t, err)
				cacheDir := filepath.Join(t.TempDir(), "cache")
				if enabled {
					config.Options.CacheDir = cacheDir
				}
				pull := func() string {
					t.Helper()
					result, err := bundle.Pull(t.Context(), ref, t.TempDir(), bundle.PullOptions{Config: config, SkipSignatureVerification: true})
					require.NoError(t, err)
					require.FileExists(t, result.OutputPath)
					return result.OutputPath
				}
				first, second := pull, create
				if createFirst {
					first, second = create, pull
				}
				before := remote.snapshot()
				first()
				cold := remote.snapshot()
				for digest := range layers {
					require.Greater(t, cold[digest], before[digest], "cold operation must download %s", digest)
				}
				first() // Same-operation reuse, independently of the next operation.
				second()
				warmArtifact := second()
				warm := remote.snapshot()
				for digest := range layers {
					if enabled {
						assert.Equal(t, cold[digest], warm[digest], "both operations must share cached layer %s", digest)
						require.FileExists(t, cacheContractBlobPath(cacheDir, digest))
					} else {
						assert.Greater(t, warm[digest], cold[digest], "empty CacheDir must disable reuse")
					}
				}
				assert.NoDirExists(t, filepath.Join(cacheDir, "images"))
				assert.NoDirExists(t, filepath.Join(cacheDir, "layers"))
				if enabled {
					require.NoError(t, filepath.WalkDir(cacheDir, func(path string, entry os.DirEntry, err error) error {
						if err != nil {
							return err
						}
						if entry.IsDir() {
							info, err := entry.Info()
							if err != nil {
								return err
							}
							assert.Zero(t, info.Mode().Perm()&0o077, "private cache directory %s has mode %o", path, info.Mode().Perm())
						}
						return nil
					}))
				} else {
					assert.NoDirExists(t, cacheDir)
				}
				// Reused bytes must still produce a complete artifact with verified packages.
				entries, err := fixtureartifact.Read(t.Context(), artifact)
				require.NoError(t, err)
				warmEntries, err := fixtureartifact.Read(t.Context(), warmArtifact)
				require.NoError(t, err)
				assert.Equal(t, entries.BlobPaths(), warmEntries.BlobPaths())
				inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: warmArtifact, Config: config})
				require.NoError(t, err)
				require.Len(t, inspected.Packages, 1)
				require.NotNil(t, inspected.Packages[0].Signature)
				assert.Equal(t, bundle.PackageVerificationStatusVerified, inspected.Packages[0].Signature.Verification)
			})
		}
	}
}

func TestCacheMetadataOnlyReuse(t *testing.T) {
	for _, tc := range []struct {
		schema  string
		enabled bool
	}{{"alpha", false}, {"alpha", true}, {"beta", false}, {"beta", true}} {
		t.Run(fmt.Sprintf("schema=%s/enabled=%t", tc.schema, tc.enabled), func(t *testing.T) {
			enabled := tc.enabled
			remote := startCacheContractRegistry(t)
			fixture := cacheContractMetadataFixture(t, tc.schema)
			layers := cacheContractPackageLayers(t, fixture.ArtifactPath)
			config := libraryRegistryConfig(fixture.Config)
			cacheDir := filepath.Join(t.TempDir(), "cache")
			if enabled {
				config.Options.CacheDir = cacheDir
			}
			ref := remote.host + "/library/metadata:v1"
			_, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
			require.NoError(t, err)
			read := func() {
				t.Helper()
				inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: ref, Config: config, SkipSignatureVerification: true})
				require.NoError(t, err)
				assert.Equal(t, "library-fixture", inspected.Name)
				expectedPackages := 2
				if tc.schema == "beta" {
					expectedPackages++
				}
				require.Len(t, inspected.Packages, expectedPackages)
			}
			read()
			cold := remote.snapshot()
			require.NotEmpty(t, cold)
			read()
			// Stop removal after metadata preparation, before any cluster access.
			result, err := bundle.Remove(t.Context(), &bundle.DeploySource{BundlePath: ref}, bundle.RemoveOptions{
				Config: config, Packages: []string{"not-in-bundle"}, Force: true, SkipSignatureVerification: true,
			})
			require.ErrorIs(t, err, bundle.ErrRemoveBundle)
			require.ErrorContains(t, err, "unknown packages")
			assert.Nil(t, result)
			warm := remote.snapshot()
			for digest, title := range layers {
				if title != "zarf.yaml" {
					assert.Zero(t, warm[digest], "metadata must not download payload %s", digest)
					assert.NoFileExists(t, cacheContractBlobPath(cacheDir, digest))
				}
			}
			if enabled {
				assert.Equal(t, cold, warm, "inspect and remove preparation must share metadata cache")
			} else {
				for digest, count := range cold {
					assert.Greater(t, warm[digest], count)
				}
				assert.NoDirExists(t, cacheDir)
			}
		})
	}
}

func TestCacheTamperedImmutableContent(t *testing.T) {
	remote := startCacheContractRegistry(t)
	fixture := createLibraryBundle(t)
	config := libraryRegistryConfig(fixture.Config)
	ref := remote.host + "/library/tampered-cache:v1"
	_, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	privateKey, publicKey := libraryKeyPair(t)
	require.NoError(t, bundle.Sign(t.Context(), bundle.SignOptions{
		Source: ref, Config: config, TmpDir: t.TempDir(),
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
	}))
	policy := bundle.VerificationPolicy{PublicKey: publicKey}
	entries, err := fixtureartifact.Read(t.Context(), fixture.ArtifactPath)
	require.NoError(t, err)
	var digest string
	for path, content := range entries.Small {
		if strings.HasPrefix(path, "oci/blobs/sha256/") && bytes.Contains(content, []byte("library-fixture")) {
			require.Empty(t, digest, "fixture must have one bundle definition blob")
			digest = "sha256:" + filepath.Base(path)
		}
	}
	require.NotEmpty(t, digest)
	for _, operation := range []string{"inspect", "verify", "pull"} {
		t.Run(operation, func(t *testing.T) {
			config.Options.CacheDir = filepath.Join(t.TempDir(), "cache")
			_, err := bundle.Pull(t.Context(), ref, t.TempDir(), bundle.PullOptions{Config: config, Verification: policy})
			require.NoError(t, err)
			_, err = bundle.Inspect(t.Context(), bundle.InspectOptions{Source: ref, Config: config, Verification: policy})
			require.NoError(t, err)
			require.NoError(t, bundle.Verify(t.Context(), bundle.VerifyOptions{Source: ref, Policy: policy, Config: config, TmpDir: t.TempDir()}))
			cold := remote.snapshot()
			path := cacheContractBlobPath(config.Options.CacheDir, digest)
			original, err := os.ReadFile(path)
			require.NoError(t, err)
			// Keep valid HCL and the original size so digest verification must detect the change.
			tampered := bytes.ReplaceAll(original, []byte("library-fixture"), []byte("library-corrupt"))
			require.NotEqual(t, original, tampered)
			require.Len(t, tampered, len(original))
			info, err := os.Stat(path)
			require.NoError(t, err)
			require.NoError(t, os.Chmod(path, 0o600))
			require.NoError(t, os.WriteFile(path, tampered, 0o600)) //nolint:gosec // G703: the generated fixture supplies the digest, and t.TempDir owns the cache root.
			require.NoError(t, os.Chmod(path, info.Mode().Perm()))
			switch operation {
			case "inspect":
				result, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: ref, Config: config, Verification: policy})
				require.ErrorIs(t, err, bundle.ErrInspectBundle)
				assert.Nil(t, result)
				require.ErrorContains(t, err, "digest")
			case "verify":
				err := bundle.Verify(t.Context(), bundle.VerifyOptions{Source: ref, Policy: policy, Config: config, TmpDir: t.TempDir()})
				require.ErrorIs(t, err, bundle.ErrVerifyBundle)
				require.ErrorContains(t, err, "digest")
			case "pull":
				output := t.TempDir()
				result, err := bundle.Pull(t.Context(), ref, output, bundle.PullOptions{Config: config, Verification: policy})
				require.ErrorIs(t, err, bundle.ErrPullBundle)
				assert.Nil(t, result)
				require.ErrorContains(t, err, "digest")
				files, err := os.ReadDir(output)
				require.NoError(t, err)
				assert.Empty(t, files, "failed pull must not publish a corrupt artifact")
			}
			assert.Equal(t, cold, remote.snapshot(), "corrupt immutable cache content must be checked on reuse")
		})
	}
}

func TestCacheLiveDiscoveryAndVerification(t *testing.T) {
	remote := startCacheContractRegistry(t)
	fixture := createLibraryBundle(t)
	config := libraryRegistryConfig(fixture.Config)
	config.Options.CacheDir = filepath.Join(t.TempDir(), "cache")
	ref := remote.host + "/library/discovery:v1"
	_, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	before, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: ref, Config: config})
	require.NoError(t, err)
	privateKey, publicKey := libraryKeyPair(t)
	policy := bundle.VerificationPolicy{PublicKey: publicKey}
	// Warm metadata must not cache the absence of signature evidence.
	require.NoError(t, bundle.Sign(t.Context(), bundle.SignOptions{
		Source: ref, Config: config, TmpDir: t.TempDir(),
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
	}))
	pull := func() {
		t.Helper()
		pulled, err := bundle.Pull(t.Context(), ref, t.TempDir(), bundle.PullOptions{Config: config, Verification: policy})
		require.NoError(t, err)
		require.NoError(t, bundle.Verify(t.Context(), bundle.VerifyOptions{Source: pulled.OutputPath, Policy: policy, Config: config, TmpDir: t.TempDir()}))
	}
	pull()
	cold := remote.snapshot()
	pull()
	assert.Equal(t, cold, remote.snapshot(), "a repeated pull must reuse all content, including signature evidence")
	verified, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: ref, Config: config, Verification: policy})
	require.NoError(t, err)
	assert.Equal(t, bundle.BundleSignatureStatusVerified, verified.BundleSignature.Status)
	_, wrongKey := libraryKeyPair(t)
	wrongPolicy := bundle.VerificationPolicy{PublicKey: wrongKey}
	failedDir := t.TempDir()
	failed, err := bundle.Pull(t.Context(), ref, failedDir, bundle.PullOptions{Config: config, Verification: wrongPolicy})
	require.ErrorIs(t, err, bundle.ErrPullBundle)
	assert.Nil(t, failed)
	outputs, err := os.ReadDir(failedDir)
	require.NoError(t, err)
	assert.Empty(t, outputs)
	_, err = bundle.Inspect(t.Context(), bundle.InspectOptions{Source: ref, Config: config, Verification: wrongPolicy})
	require.ErrorIs(t, err, bundle.ErrInspectBundle)
	assert.Equal(t, cold, remote.snapshot(), "warm verification must reuse bytes, not a previous trust decision")

	// Re-signing discovers existing evidence live but reads its immutable bytes
	// through the same cache. Publication must not bypass that reader.
	err = bundle.Sign(t.Context(), bundle.SignOptions{
		Source: ref, Config: config, TmpDir: t.TempDir(),
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
	})
	require.ErrorIs(t, err, bundle.ErrSignBundle)
	require.ErrorContains(t, err, "already exists")
	assert.Equal(t, cold, remote.snapshot(), "re-signing must reuse cached signature manifests and evidence")
	require.NoError(t, bundle.Sign(t.Context(), bundle.SignOptions{
		Source: ref, Config: config, TmpDir: t.TempDir(),
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey, Overwrite: true},
	}))

	// The same tag can move while a digest-pinned reference remains stable.
	second := createLibraryBundleWithoutOptionalComponent(t)
	_, err = bundle.Push(t.Context(), second.ArtifactPath, ref, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	after, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: ref, Config: config, SkipSignatureVerification: true})
	require.NoError(t, err)
	assert.NotEqual(t, before.ArtifactDigest, after.ArtifactDigest)
	assert.Empty(t, after.Bundle.Packages[1].OptionalComponents)
	pinned := strings.TrimSuffix(ref, ":v1") + "@" + before.ArtifactDigest
	old, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: pinned, Config: config, SkipSignatureVerification: true})
	require.NoError(t, err)
	assert.Equal(t, before.ArtifactDigest, old.ArtifactDigest)
	assert.Equal(t, []string{"optional"}, old.Bundle.Packages[1].OptionalComponents)
}

func TestCacheSignReconfigureMetadataReuse(t *testing.T) {
	remote := startCacheContractRegistry(t)
	fixture := createLibraryBundle(t)
	layers := cacheContractPackageLayers(t, fixture.ArtifactPath)
	config := libraryRegistryConfig(fixture.Config)
	config.Options.CacheDir = filepath.Join(t.TempDir(), "cache")
	ref := remote.host + "/library/metadata-writers:v1"
	_, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	_, err = bundle.Inspect(t.Context(), bundle.InspectOptions{Source: ref, Config: config, SkipSignatureVerification: true})
	require.NoError(t, err)
	cold := remote.snapshot()
	require.NotEmpty(t, cold)
	privateKey, _ := libraryKeyPair(t)
	require.NoError(t, bundle.Sign(t.Context(), bundle.SignOptions{
		Source: ref, Config: config, TmpDir: t.TempDir(),
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
	}))
	defaults := writeLibraryDefaults(t, `variables = { fixture = "cached-reconfigure" }`)
	reconfigured, err := bundle.Reconfigure(t.Context(), ref, defaults, bundle.ReconfigureOptions{
		Config: config, Suffix: "-cached", SkipSignatureVerification: true,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, reconfigured.OCIReference)
	warm := remote.snapshot()
	for digest, count := range cold {
		assert.Equal(t, count, warm[digest], "sign and reconfigure must reuse cached source metadata %s", digest)
	}
	for digest, title := range layers {
		if title != "zarf.yaml" {
			assert.Zero(t, warm[digest], "metadata writers must not download package payloads")
			assert.NoFileExists(t, cacheContractBlobPath(config.Options.CacheDir, digest))
		}
	}
}

func TestCacheLocalSourcesLeaveRemoteCacheUntouched(t *testing.T) {
	fixture := createLibraryBundle(t)
	fixture.Config.Options.CacheDir = filepath.Join(t.TempDir(), "cache")
	created, err := bundle.Create(t.Context(), fixture.BundleFile, bundle.CreateOptions{
		Config: fixture.Config, Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.NoError(t, err)
	_, err = bundle.Inspect(t.Context(), bundle.InspectOptions{Source: created.OutputPath, Config: fixture.Config})
	require.NoError(t, err)
	_, err = bundle.Remove(t.Context(), &bundle.DeploySource{BundlePath: created.OutputPath}, bundle.RemoveOptions{
		Config: fixture.Config, Packages: []string{"not-in-bundle"}, Force: true, SkipSignatureVerification: true,
	})
	require.ErrorIs(t, err, bundle.ErrRemoveBundle)
	require.ErrorContains(t, err, "unknown packages")
	assert.NoDirExists(t, fixture.Config.Options.CacheDir)
}

func TestCacheConcurrentPublicOperations(t *testing.T) {
	remote := startCacheContractRegistry(t)
	privateKey, publicKey := libraryKeyPair(t)
	pkg := createLibraryPackageWithSigning(t, t.TempDir(), "concurrent-cache", privateKey)
	packageRef := "oci://" + publishLibraryPackage(t, pkg, remote.host)
	bootstrap := libraryRegistryConfig(libraryFixtureConfig(t))
	file := writePackageVerificationBundle(t, t.TempDir(), packageRef, publicKey)
	created, err := bundle.Create(t.Context(), file, bundle.CreateOptions{
		Config: bootstrap, Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.NoError(t, err)
	layers := cacheContractPackageLayers(t, created.OutputPath)
	ref := remote.host + "/library/concurrent-cache:v1"
	_, err = bundle.Push(t.Context(), created.OutputPath, ref, bundle.PushOptions{Config: bootstrap})
	require.NoError(t, err)
	require.NoError(t, bundle.Sign(t.Context(), bundle.SignOptions{
		Source: ref, Config: bootstrap, TmpDir: t.TempDir(),
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
	}))
	policy := bundle.VerificationPolicy{PublicKey: publicKey}
	cacheDir := filepath.Join(t.TempDir(), "shared-cache")
	require.NoDirExists(t, cacheDir, "fixture setup must leave the shared cache cold")

	run := func() {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		group, ctx := errgroup.WithContext(ctx)
		start := make(chan struct{})
		for range 3 {
			for _, operation := range []string{"create", "pull", "inspect", "verify"} {
				// Each operation represents an independent library caller. Only the
				// cache directory and immutable fixture inputs are shared.
				config := libraryRegistryConfig(libraryFixtureConfig(t))
				config.Options.CacheDir = cacheDir
				output := t.TempDir()
				bundleFile := writePackageVerificationBundle(t, t.TempDir(), packageRef, publicKey)
				group.Go(func() error {
					select {
					case <-start:
					case <-ctx.Done():
						return ctx.Err()
					}
					var artifact string
					switch operation {
					case "create":
						result, err := bundle.Create(ctx, bundleFile, bundle.CreateOptions{
							Config: config, Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
						})
						if err != nil {
							return fmt.Errorf("concurrent create: %w", err)
						}
						artifact = result.OutputPath
					case "pull":
						result, err := bundle.Pull(ctx, ref, output, bundle.PullOptions{Config: config, Verification: policy})
						if err != nil {
							return fmt.Errorf("concurrent pull: %w", err)
						}
						artifact = result.OutputPath
					case "inspect":
						result, err := bundle.Inspect(ctx, bundle.InspectOptions{Source: ref, Config: config, Verification: policy})
						if err != nil {
							return fmt.Errorf("concurrent inspect: %w", err)
						}
						if result.Name != "package-verification" || len(result.Packages) != 1 {
							return fmt.Errorf("concurrent inspect returned unexpected bundle: %+v", result)
						}
					case "verify":
						if err := bundle.Verify(ctx, bundle.VerifyOptions{Source: ref, Config: config, Policy: policy, TmpDir: output}); err != nil {
							return fmt.Errorf("concurrent verify: %w", err)
						}
					}
					if artifact != "" {
						result, err := bundle.Inspect(ctx, bundle.InspectOptions{Source: artifact, Config: config})
						if err != nil {
							return fmt.Errorf("inspecting concurrent %s output: %w", operation, err)
						}
						if len(result.Packages) != 1 || result.Packages[0].Signature == nil || result.Packages[0].Signature.Verification != bundle.PackageVerificationStatusVerified {
							return fmt.Errorf("concurrent %s output lacks a verified package", operation)
						}
					}
					return nil
				})
			}
		}
		close(start)
		require.NoError(t, group.Wait())
	}

	before := remote.snapshot()
	run()
	cold := remote.snapshot()
	for digest := range layers {
		require.Greater(t, cold[digest], before[digest], "cold operations must download %s", digest)
		require.FileExists(t, cacheContractBlobPath(cacheDir, digest))
	}
	run()
	warm := remote.snapshot()
	// Package-root resolution and signature discovery remain live. Package
	// metadata and payload layers must be reused by concurrent create and pull.
	for digest := range layers {
		assert.Equal(t, cold[digest], warm[digest], "concurrent warm operations must reuse cached layer %s", digest)
	}
}

func TestCacheConcurrentCreatesFromRemote(t *testing.T) {
	const writers = 8
	privateKey, publicKey := libraryKeyPair(t)
	pkg := createLibraryPackageWithSigning(t, t.TempDir(), "concurrent-create", privateKey)
	pkgLayout, err := packager.LoadPackage(t.Context(), pkg, packager.LoadOptions{CachePath: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
	manifest, err := pkgLayout.Manifest()
	require.NoError(t, err)
	layer := manifest.Locate("components/main.tar")
	require.NotEmpty(t, layer.Digest)

	var downloads atomic.Int32
	release := make(chan struct{})
	handler := registry.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/blobs/"+layer.Digest.String()) {
			// Hold every response until all creates have missed the same cached
			// payload. Releasing them together forces competing cache writes.
			if downloads.Add(1) == writers {
				close(release)
			}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	packageRef := "oci://" + publishLibraryPackage(t, pkg, strings.TrimPrefix(server.URL, "http://"))
	cacheDir := filepath.Join(t.TempDir(), "shared-cache")
	require.NoDirExists(t, cacheDir)

	run := func() {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		group, ctx := errgroup.WithContext(ctx)
		start := make(chan struct{})
		for i := range writers {
			config := libraryRegistryConfig(libraryFixtureConfig(t))
			config.Options.CacheDir = cacheDir
			file := writePackageVerificationBundle(t, t.TempDir(), packageRef, publicKey)
			group.Go(func() error {
				select {
				case <-start:
				case <-ctx.Done():
					return ctx.Err()
				}
				result, err := bundle.Create(ctx, file, bundle.CreateOptions{
					Config: config, Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
				})
				if err != nil {
					return fmt.Errorf("remote create %d: %w", i, err)
				}
				inspected, err := bundle.Inspect(ctx, bundle.InspectOptions{Source: result.OutputPath, Config: config})
				if err != nil {
					return fmt.Errorf("inspecting create %d output: %w", i, err)
				}
				if inspected.Name != "package-verification" || len(inspected.Packages) != 1 || inspected.Packages[0].Signature == nil || inspected.Packages[0].Signature.Verification != bundle.PackageVerificationStatusVerified {
					return fmt.Errorf("remote create %d produced an invalid or unverified bundle", i)
				}
				return nil
			})
		}
		close(start)
		require.NoError(t, group.Wait())
	}

	run()
	require.EqualValues(t, writers, downloads.Load(), "all creates must reach the same cold-cache miss before any payload response is released")
	data, err := os.ReadFile(cacheContractBlobPath(cacheDir, layer.Digest.String()))
	require.NoError(t, err)
	require.Len(t, data, int(layer.Size))
	verifier := layer.Digest.Verifier()
	_, err = verifier.Write(data)
	require.NoError(t, err)
	assert.True(t, verifier.Verified(), "competing writes must leave a valid cached payload")
	run()
	assert.EqualValues(t, writers, downloads.Load(), "fresh concurrent creates must reuse the cached payload")
}

func cacheContractMetadataFixture(t *testing.T, schema string) libraryBundleFixture {
	t.Helper()
	fixture := createLibraryBundle(t)
	if schema == "alpha" {
		return fixture
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "payload.txt"), []byte("beta payload\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "zarf.yaml"), []byte(`apiVersion: zarf.dev/v1beta1
kind: ZarfPackageConfig
metadata:
  name: beta
  version: 1.0.0
components:
  - name: main
    files:
      - source: payload.txt
        destination: /tmp/cache-beta.txt
`), 0o600))
	resolved, err := load.Package(t.Context(), root, load.PackageOptions{})
	require.NoError(t, err)
	layout, err := assemble.AssemblePackage(t.Context(), resolved, assemble.AssembleOptions{SkipSBOM: true, CachePath: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, layout.Cleanup()) })
	packagePath, err := layout.Archive(t.Context(), root, 0)
	require.NoError(t, err)
	definition, err := os.ReadFile(fixture.BundleFile)
	require.NoError(t, err)
	definition = append(definition, []byte(fmt.Sprintf(`
package "beta" {
  source = %q
  signature_verification { verify = false }
}
`, packagePath))...)
	require.NoError(t, os.WriteFile(fixture.BundleFile, definition, 0o600)) //nolint:gosec // G703: createLibraryBundle generates this path inside t.TempDir.
	created, err := bundle.Create(t.Context(), fixture.BundleFile, bundle.CreateOptions{
		Config: fixture.Config, Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.NoError(t, err)
	fixture.ArtifactPath = created.OutputPath
	return fixture
}

// Count content GETs, not elapsed time. Tag and signature discovery remain live.
type cacheContractRegistry struct {
	host      string
	mu        sync.Mutex
	downloads map[string]int
}

func startCacheContractRegistry(t *testing.T) *cacheContractRegistry {
	t.Helper()
	remote := &cacheContractRegistry{downloads: map[string]int{}}
	handler := registry.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && (strings.Contains(r.URL.Path, "/blobs/") || strings.Contains(r.URL.Path, "/manifests/sha256:")) {
			remote.mu.Lock()
			remote.downloads[filepath.Base(r.URL.Path)]++
			remote.mu.Unlock()
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	remote.host = strings.TrimPrefix(server.URL, "http://")
	return remote
}

func (r *cacheContractRegistry) snapshot() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[string]int, len(r.downloads))
	for digest, count := range r.downloads {
		result[digest] = count
	}
	return result
}

func cacheContractPackageLayers(t *testing.T, artifact string) map[string]string {
	t.Helper()
	entries, err := fixtureartifact.Read(t.Context(), artifact)
	require.NoError(t, err)
	manifests, err := entries.PackageManifestDigests()
	require.NoError(t, err)
	layers := map[string]string{}
	for digest := range manifests {
		var manifest ocispec.Manifest
		require.NoError(t, json.Unmarshal(entries.Small["oci/blobs/sha256/"+strings.TrimPrefix(digest, "sha256:")], &manifest))
		for _, layer := range manifest.Layers {
			layers[layer.Digest.String()] = layer.Annotations[ocispec.AnnotationTitle]
		}
	}
	require.NotEmpty(t, layers)
	payloads := 0
	for _, title := range layers {
		if title != "zarf.yaml" {
			payloads++
		}
	}
	require.Positive(t, payloads, "fixture must contain payload layers to test metadata-only reads")
	return layers
}

func cacheContractBlobPath(cacheDir, digest string) string {
	return filepath.Join(cacheDir, "bundle-oci", "images", "blobs", "sha256", strings.TrimPrefix(digest, "sha256:"))
}
