// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	fixtureartifact "github.com/defenseunicorns/uds-cli/tests/fixtures/artifact"
	"github.com/google/go-containerregistry/pkg/registry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRemoteMetadataCacheFallback checks that inspect and remove preparation
// recover from a broken cache without downloading package payloads. Removal
// deliberately stops at package selection, before any cluster access or actions.
func TestRemoteMetadataCacheFallback(t *testing.T) {
	for _, operation := range []string{"inspect", "inspect skipping verification", "verified remove preparation"} {
		for _, phase := range []string{"initialization", "population"} {
			t.Run(operation+"/"+phase, func(t *testing.T) {
				fixture := createLibraryBundle(t)
				privateKey, publicKey := libraryKeyPair(t)
				require.NoError(t, bundle.Sign(t.Context(), bundle.SignOptions{
					Source: fixture.ArtifactPath, Config: fixture.Config, TmpDir: t.TempDir(),
					Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
				}))
				entries, err := fixtureartifact.Read(t.Context(), fixture.ArtifactPath)
				require.NoError(t, err)
				manifests, err := entries.PackageManifestDigests()
				require.NoError(t, err)
				payloads := make(map[string]bool)
				for digest := range manifests {
					var manifest ocispec.Manifest
					require.NoError(t, json.Unmarshal(entries.Small["oci/blobs/sha256/"+strings.TrimPrefix(digest, "sha256:")], &manifest))
					for _, layer := range manifest.Layers {
						if layer.Annotations[ocispec.AnnotationTitle] != "zarf.yaml" {
							payloads[layer.Digest.String()] = true
						}
					}
				}
				require.NotEmpty(t, payloads)
				var payloadDownloads atomic.Int64
				handler := registry.New()
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") && payloads[filepath.Base(r.URL.Path)] {
						payloadDownloads.Add(1)
						http.Error(w, "package payloads must not be downloaded", http.StatusForbidden)
						return
					}
					handler.ServeHTTP(w, r)
				}))
				t.Cleanup(server.Close)
				config := libraryRegistryConfig(fixture.Config)
				ref := strings.TrimPrefix(server.URL, "http://") + "/library/fallback:v1"
				_, err = bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
				require.NoError(t, err)
				cacheDir := t.TempDir()
				blockedPath := filepath.Join(cacheDir, "bundle-oci", "images")
				if phase == "population" {
					blockedPath = filepath.Join(blockedPath, "ingest")
				}
				require.NoError(t, os.MkdirAll(filepath.Dir(blockedPath), 0o700))
				require.NoError(t, os.WriteFile(blockedPath, []byte("cache obstruction"), 0o600))
				config.Options.CacheDir = cacheDir
				streams, _, out, errOut := iostreams.NewTestIOStreams()
				if operation == "verified remove preparation" {
					result, err := bundle.Remove(t.Context(), &bundle.DeploySource{BundlePath: ref}, bundle.RemoveOptions{
						Config: config, Verification: bundle.VerificationPolicy{PublicKey: publicKey}, Streams: streams,
						Packages: []string{"not-in-bundle"}, Force: true,
					})
					require.ErrorIs(t, err, bundle.ErrRemoveBundle)
					require.ErrorContains(t, err, "unknown packages", "verification and metadata preparation must succeed before selection stops removal")
					assert.Nil(t, result)
					// Cache recovery must not allow removal to bypass the
					// caller's trust policy. A wrong key still stops preparation.
					_, wrongKey := libraryKeyPair(t)
					_, err = bundle.Remove(t.Context(), &bundle.DeploySource{BundlePath: ref}, bundle.RemoveOptions{
						Config: config, Verification: bundle.VerificationPolicy{PublicKey: wrongKey},
						Packages: []string{"not-in-bundle"}, Force: true,
					})
					require.ErrorContains(t, err, "verifying bundle signature")
					assert.NotContains(t, err.Error(), "unknown packages")
				} else {
					result, err := bundle.Inspect(t.Context(), bundle.InspectOptions{
						Source: ref, Config: config, Streams: streams, SkipSignatureVerification: operation == "inspect skipping verification",
					})
					require.NoError(t, err)
					require.NotNil(t, result)
					assert.Equal(t, "library-fixture", result.Name)
					assert.Len(t, result.Packages, 2)
				}
				assert.Equal(t, 1, strings.Count(errOut.String(), "retrying read once without caching"))
				assert.Empty(t, out.String())
				assert.Zero(t, payloadDownloads.Load())
				assert.Equal(t, cacheDir, config.Options.CacheDir)
				marker, err := os.ReadFile(blockedPath)
				require.NoError(t, err)
				assert.Equal(t, "cache obstruction", string(marker))
				temporary, err := os.ReadDir(config.Options.TmpDir)
				require.NoError(t, err)
				assert.Empty(t, temporary)
			})
		}
	}
}

// TestMetadataCacheDoesNotRetryVerificationFailures ensures a wrong or missing
// key is not treated as a cache failure, even when the key path is in the cache.
func TestMetadataCacheDoesNotRetryVerificationFailures(t *testing.T) {
	fixture, ref, _ := publishSignedLibraryBundle(t, startLibraryRegistry(t))
	config := libraryRegistryConfig(fixture.Config)
	config.Options.CacheDir = t.TempDir()
	_, wrongKey := libraryKeyPair(t)
	for _, key := range []string{wrongKey, filepath.Join(config.Options.CacheDir, "missing-key.pub")} {
		t.Run(filepath.Base(key), func(t *testing.T) {
			streams, _, _, errOut := iostreams.NewTestIOStreams()
			result, err := bundle.Remove(t.Context(), &bundle.DeploySource{BundlePath: ref}, bundle.RemoveOptions{
				Config: config, Verification: bundle.VerificationPolicy{PublicKey: key}, Streams: streams,
				Packages: []string{"not-in-bundle"}, Force: true,
			})
			require.ErrorIs(t, err, bundle.ErrRemoveBundle)
			require.ErrorContains(t, err, "verifying bundle signature")
			assert.NotContains(t, err.Error(), "unknown packages")
			assert.NotContains(t, errOut.String(), "retrying")
			assert.Nil(t, result)
		})
	}
}
