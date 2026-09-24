// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cli

package bundle_test

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	clibundle "github.com/defenseunicorns/uds-cli/internal/cli/bundle"
	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignVerifyCommand_Integration(t *testing.T) {
	artifactPath := createInspectArtifact(t)
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
