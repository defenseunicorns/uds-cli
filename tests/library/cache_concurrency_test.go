// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

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
