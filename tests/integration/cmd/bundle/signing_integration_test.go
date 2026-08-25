// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package bundle_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	clibundle "github.com/defenseunicorns/uds-cli/internal/cli/bundle"
	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignVerifyCommand_Integration(t *testing.T) {
	artifactPath := testutil.CreateBundleFromTestData(t, "bundles/create/init", runtime.GOARCH)
	privateKey, publicKey := testutil.GenerateCosignKeyPair(t)

	signStreams, _, _, _ := iostreams.NewTestIOStreams()
	sign := clibundle.NewBundleCommand(signStreams)
	sign.SetArgs([]string{"sign", artifactPath, "--signing-key", privateKey})
	require.NoError(t, sign.Execute())

	verifyStreams, _, _, _ := iostreams.NewTestIOStreams()
	verify := clibundle.NewBundleCommand(verifyStreams)
	verify.SetArgs([]string{"verify", artifactPath, "--public-key", publicKey})
	require.NoError(t, verify.Execute())

	inspectStreams, _, inspectOut, _ := iostreams.NewTestIOStreams()
	inspect := clibundle.NewBundleCommand(inspectStreams)
	inspect.SetArgs([]string{"inspect", artifactPath, "--public-key", publicKey, "--output", "json"})
	require.NoError(t, inspect.Execute())

	var inspectResult bundlepkg.InspectResult
	require.NoError(t, json.Unmarshal(inspectOut.Bytes(), &inspectResult))
	require.NotNil(t, inspectResult.BundleSignature)
	assert.Equal(t, bundlepkg.BundleSignatureStatusVerified, inspectResult.BundleSignature.Status)

	workspace := t.TempDir()
	require.NoError(t, artifact.ExtractTarZst(t.Context(), iostreams.IOStreams{}, artifactPath, workspace))
	indexPath := filepath.Join(workspace, "oci", "index.json")
	index, err := os.ReadFile(indexPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(indexPath, append(index, '\n'), 0o600))

	tampered := filepath.Join(t.TempDir(), "tampered.tar.zst")
	require.NoError(t, artifact.WriteTarZst(t.Context(), iostreams.IOStreams{}, tampered, workspace))
	err = bundlepkg.Verify(t.Context(), bundlepkg.VerifyOptions{
		Source: tampered,
		Policy: bundlepkg.VerificationPolicy{PublicKey: readFile(t, publicKey)},
		TmpDir: t.TempDir(),
	})
	require.ErrorContains(t, err, "verifying bundle signature")
}

func TestSignedOCIPull_Integration(t *testing.T) {
	arch := runtime.GOARCH
	registryHost := testutil.StartLocalRegistry(t)
	ref := fmt.Sprintf("%s/test/signed-bundle:v0.1.0", registryHost)
	artifactPath := createInspectArtifact(t)
	privateKey, publicKey := testutil.GenerateCosignKeyPair(t)

	signStreams, _, _, _ := iostreams.NewTestIOStreams()
	sign := clibundle.NewBundleCommand(signStreams)
	sign.SetArgs([]string{"sign", artifactPath, "--signing-key", privateKey})
	require.NoError(t, sign.Execute())

	config := &bundlepkg.UDSBundleConfig{
		Global:  &bundlepkg.GlobalOptions{},
		Options: &bundlepkg.ConfigOptions{TmpDir: t.TempDir(), PlainHTTP: true, Architecture: arch, Concurrency: 10},
	}
	_, err := bundlepkg.Push(t.Context(), artifactPath, ref, bundlepkg.PushOptions{Config: config})
	require.NoError(t, err)

	_, err = bundlepkg.Pull(t.Context(), ref, t.TempDir(), bundlepkg.PullOptions{Config: config})
	require.ErrorContains(t, err, "signature verification must configure exactly one of public key or keyless")

	pullResult, err := bundlepkg.Pull(t.Context(), ref, t.TempDir(), bundlepkg.PullOptions{
		Config:       config,
		Verification: bundlepkg.VerificationPolicy{PublicKey: readFile(t, publicKey)},
	})
	require.NoError(t, err)
	require.FileExists(t, pullResult.OutputPath)
	assertValidBundleStructure(t, pullResult.OutputPath)
}

func TestKeylessSignVerify_Integration(t *testing.T) {
	githubActions := os.Getenv("GITHUB_ACTIONS") == "true"
	idTokenRequestURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	idTokenRequestToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	repository := os.Getenv("GITHUB_REPOSITORY")
	ref := os.Getenv("GITHUB_REF")

	if !githubActions {
		t.Skip("keyless signing requires GitHub Actions OIDC credentials")
	}

	require.NotEmpty(t, idTokenRequestURL, "keyless signing requires id-token: write permissions in GitHub Actions")
	require.NotEmpty(t, idTokenRequestToken, "keyless signing requires id-token: write permissions in GitHub Actions")
	require.NotEmpty(t, repository, "keyless verification requires GITHUB_REPOSITORY in GitHub Actions")
	require.NotEmpty(t, ref, "keyless verification requires GITHUB_REF in GitHub Actions")

	artifactPath := createInspectArtifact(t)
	serverURL := os.Getenv("GITHUB_SERVER_URL")
	if serverURL == "" {
		serverURL = "https://github.com"
	}
	identityRegexp := fmt.Sprintf(
		"^%s/%s/\\.github/workflows/[^@]+@%s$",
		regexp.QuoteMeta(strings.TrimRight(serverURL, "/")),
		regexp.QuoteMeta(strings.TrimLeft(repository, "/")),
		regexp.QuoteMeta(ref),
	)

	signStreams, _, _, _ := iostreams.NewTestIOStreams()
	sign := clibundle.NewBundleCommand(signStreams)
	sign.SetArgs([]string{"sign", artifactPath, "--keyless"})
	require.NoError(t, sign.Execute())

	verifyStreams, _, _, _ := iostreams.NewTestIOStreams()
	verify := clibundle.NewBundleCommand(verifyStreams)
	verify.SetArgs([]string{
		"verify", artifactPath,
		"--certificate-identity-regexp", identityRegexp,
		"--certificate-oidc-issuer", "https://token.actions.githubusercontent.com",
	})
	require.NoError(t, verify.Execute())
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(contents)
}
