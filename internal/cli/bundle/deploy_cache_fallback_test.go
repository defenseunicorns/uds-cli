// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/sigstore/cosign/v3/pkg/cosign"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeployOptions_Run_OCICacheFallback checks the real OCI pull path through
// deploy. A broken cache must still deliver a verified archive to the runner;
// an action error must return without rerunning the pull or the action.
func TestDeployOptions_Run_OCICacheFallback(t *testing.T) {
	for _, phase := range []string{"initialization", "population"} {
		t.Run(phase, func(t *testing.T) {
			ref, publicKey := deployCacheFallbackArtifact(t, true)
			publicKeyBytes, err := os.ReadFile(publicKey)
			require.NoError(t, err)
			cacheDir, tmpDir, blockedPath := obstructDeployCache(t, phase)
			for _, scenario := range []string{"success", "deployment error", "wrong verification key"} {
				t.Run(scenario, func(t *testing.T) {
					streams, _, out, errOut := iostreams.NewTestIOStreams()
					o := deployCacheFallbackOptions(t, streams, ref, cacheDir, tmpDir)
					verificationKey := publicKey
					if scenario == "wrong verification key" {
						keys, err := cosign.GenerateKeyPair(nil)
						require.NoError(t, err)
						verificationKey = filepath.Join(t.TempDir(), "wrong.pub")
						require.NoError(t, os.WriteFile(verificationKey, keys.PublicBytes, 0o600))
					}
					o.Verification.PublicKey = verificationKey
					pullCalls, runnerCalls := 0, 0
					o.pullBundle = func(ctx context.Context, ref, dir string, opts bundle.PullOptions) (*bundle.PullResult, error) {
						pullCalls++
						return bundle.Pull(ctx, ref, dir, opts)
					}
					deployErr := errors.New("deployment action failed")
					o.runDeploy = func(ctx context.Context, streams iostreams.IOStreams, cfg *bundle.UDSBundleConfig, opts deployOptions) (*bundle.DeployResult, error) {
						runnerCalls++
						assert.Equal(t, cacheDir, cfg.Options.CacheDir)
						require.FileExists(t, opts.bundlePath)
						require.NoError(t, bundle.Verify(ctx, bundle.VerifyOptions{
							Source: opts.bundlePath, Config: cfg,
							Policy: bundle.VerificationPolicy{PublicKey: string(publicKeyBytes)}, TmpDir: tmpDir, Streams: streams,
						}))
						if scenario == "deployment error" {
							return nil, deployErr
						}
						return &bundle.DeployResult{BundleName: "inspect-test"}, nil
					}
					err := o.Run(t.Context())
					switch scenario {
					case "success":
						require.NoError(t, err)
						assert.Contains(t, out.String(), "inspect-test")
						assert.Equal(t, 1, runnerCalls)
					case "deployment error":
						require.ErrorIs(t, err, deployErr)
						assert.Empty(t, out.String())
						assert.Equal(t, 1, runnerCalls, "deployment actions must not be retried")
					case "wrong verification key":
						require.ErrorIs(t, err, bundle.ErrPullBundle)
						require.ErrorContains(t, err, "verifying bundle signature")
						assert.Empty(t, out.String())
						assert.Zero(t, runnerCalls, "verification must finish before deployment starts")
					}
					assert.Equal(t, 1, pullCalls, "the deploy command calls Pull once; Pull owns its cache retry")
					assert.Equal(t, 1, strings.Count(errOut.String(), "retrying pull once without caching"))
					assert.Equal(t, cacheDir, o.Config.Options.CacheDir)
					assertDeployCacheUntouched(t, tmpDir, blockedPath)
				})
			}
		})
	}
}

// TestDeployOptions_Run_MissingPolicyCacheFallback exercises the preliminary
// metadata check used only when no trust policy was supplied. A broken cache
// must not hide the signed/unsigned guidance or start a pull or deployment.
func TestDeployOptions_Run_MissingPolicyCacheFallback(t *testing.T) {
	for _, signed := range []bool{true, false} {
		t.Run(map[bool]string{true: "signed", false: "unsigned"}[signed], func(t *testing.T) {
			ref, _ := deployCacheFallbackArtifact(t, signed)
			cacheDir, tmpDir, blockedPath := obstructDeployCache(t, "population")
			streams, _, out, errOut := iostreams.NewTestIOStreams()
			o := deployCacheFallbackOptions(t, streams, ref, cacheDir, tmpDir)
			pullCalls, runnerCalls := 0, 0
			o.pullBundle = func(context.Context, string, string, bundle.PullOptions) (*bundle.PullResult, error) {
				pullCalls++
				return nil, errors.New("unexpected pull")
			}
			o.runDeploy = func(context.Context, iostreams.IOStreams, *bundle.UDSBundleConfig, deployOptions) (*bundle.DeployResult, error) {
				runnerCalls++
				return nil, errors.New("unexpected deployment")
			}
			err := o.Run(t.Context())
			if signed {
				require.ErrorContains(t, err, "signature verification must configure exactly one of public key or keyless")
			} else {
				require.ErrorContains(t, err, "bundle is not signed")
				require.ErrorContains(t, err, "--skip-signature-verification")
			}
			assert.NotContains(t, err.Error(), "checking bundle signature")
			assert.Empty(t, out.String())
			assert.Equal(t, 1, strings.Count(errOut.String(), "retrying read once without caching"))
			assert.Zero(t, pullCalls)
			assert.Zero(t, runnerCalls)
			assert.Equal(t, cacheDir, o.Config.Options.CacheDir)
			assertDeployCacheUntouched(t, tmpDir, blockedPath)
		})
	}
}

func deployCacheFallbackArtifact(t *testing.T, signed bool) (string, string) {
	t.Helper()
	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)
	artifactPath := createTestArtifact(t)
	config := testInspectConfig()
	config.Options.PlainHTTP = true
	config.Options.TmpDir = t.TempDir()
	publicKey := ""
	if signed {
		keys, err := cosign.GenerateKeyPair(nil)
		require.NoError(t, err)
		keyDir := t.TempDir()
		privateKey := filepath.Join(keyDir, "cosign.key")
		publicKey = filepath.Join(keyDir, "cosign.pub")
		require.NoError(t, os.WriteFile(privateKey, keys.PrivateBytes, 0o600))
		require.NoError(t, os.WriteFile(publicKey, keys.PublicBytes, 0o600))
		require.NoError(t, bundle.Sign(t.Context(), bundle.SignOptions{
			Source: artifactPath, Config: config, TmpDir: config.Options.TmpDir,
			Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
		}))
	}
	ref := "oci://" + strings.TrimPrefix(server.URL, "http://") + "/test/deploy-fallback:v1"
	_, err := bundle.Push(t.Context(), artifactPath, ref, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	return ref, publicKey
}

func obstructDeployCache(t *testing.T, phase string) (string, string, string) {
	t.Helper()
	cacheDir, tmpDir := t.TempDir(), t.TempDir()
	blockedPath := filepath.Join(cacheDir, "bundle-oci", "images")
	if phase == "population" {
		blockedPath = filepath.Join(blockedPath, "ingest")
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(blockedPath), 0o700))
	require.NoError(t, os.WriteFile(blockedPath, []byte("cache obstruction"), 0o600))
	return cacheDir, tmpDir, blockedPath
}

func deployCacheFallbackOptions(t *testing.T, streams iostreams.IOStreams, ref, cacheDir, tmpDir string) *DeployOptions {
	t.Helper()
	root := NewBundleCommand(streams)
	cmd, _, err := root.Find([]string{"deploy"})
	require.NoError(t, err)
	o := NewDeployOptions(streams)
	require.NoError(t, o.Complete(cmd, []string{ref}))
	o.flags = CLIFlags{
		CacheDir: cacheDir, CacheDirChanged: true,
		TmpDir: tmpDir, TmpDirChanged: true,
		PlainHTTP: true, PlainHTTPChanged: true,
	}
	return o
}

func assertDeployCacheUntouched(t *testing.T, tmpDir, blockedPath string) {
	t.Helper()
	marker, err := os.ReadFile(blockedPath)
	require.NoError(t, err)
	assert.Equal(t, "cache obstruction", string(marker))
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "temporary deploy and pull workspaces must be cleaned")
}
