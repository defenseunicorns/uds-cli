// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library && signing_integration

package bundle_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeylessBundleSigning(t *testing.T) {
	require.Equal(t, "true", os.Getenv("GITHUB_ACTIONS"), "keyless signing requires GitHub Actions OIDC credentials")
	require.NotEmpty(t, os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"), "keyless signing requires id-token: write permissions")
	require.NotEmpty(t, os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"), "keyless signing requires id-token: write permissions")
	repository := os.Getenv("GITHUB_REPOSITORY")
	ref := os.Getenv("GITHUB_REF")
	require.NotEmpty(t, repository, "keyless verification requires GITHUB_REPOSITORY")
	require.NotEmpty(t, ref, "keyless verification requires GITHUB_REF")

	fixture := createLibraryArtifact(t)
	policy := keylessVerificationPolicy(t, repository, ref)
	require.NoError(t, bundle.Sign(t.Context(), bundle.SignOptions{
		Source: fixture,
		Signing: bundle.SigningOptions{
			Mode: bundle.SigningModeKeyless,
		},
		TmpDir: t.TempDir(),
	}))
	require.NoError(t, bundle.Verify(t.Context(), bundle.VerifyOptions{
		Source: fixture,
		Policy: policy,
		TmpDir: t.TempDir(),
	}))

	config := libraryFixtureConfig(t)
	config.SignatureVerification = &policy
	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: fixture, Config: config})
	require.NoError(t, err)
	require.NotNil(t, inspected.BundleSignature)
	assert.Equal(t, bundle.BundleSignatureStatusVerified, inspected.BundleSignature.Status)

	wrongPolicy := policy
	wrongPolicy.Keyless = &bundle.KeylessVerification{
		CertificateIdentity:   "https://example.invalid/workflow@refs/heads/wrong",
		CertificateOIDCIssuer: "https://token.actions.githubusercontent.com",
	}
	err = bundle.Verify(t.Context(), bundle.VerifyOptions{
		Source: fixture,
		Policy: wrongPolicy,
		TmpDir: t.TempDir(),
	})
	require.ErrorIs(t, err, bundle.ErrVerifyBundle)
}

func TestKeylessPackageVerification(t *testing.T) {
	validDir := copyKeylessFixture(t, "init-keyless")
	validConfig := libraryFixtureConfig(t)
	created, err := bundle.Create(t.Context(), filepath.Join(validDir, libraryBundleFileName), bundle.CreateOptions{
		Config:  validConfig,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.NoError(t, err)
	require.NotNil(t, created)

	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: created.OutputPath, Config: validConfig})
	require.NoError(t, err)
	require.NotNil(t, inspected.BundleSignature)
	assert.Equal(t, bundle.BundleSignatureStatusNotChecked, inspected.BundleSignature.Status)
	require.Len(t, inspected.Packages, 1)
	require.NotNil(t, inspected.Packages[0].Signature)
	assert.Equal(t, bundle.PackageSigningStatusSigned, inspected.Packages[0].Signature.Signed)
	assert.Equal(t, bundle.PackageVerificationStatusVerified, inspected.Packages[0].Signature.Verification)

	invalidDir := copyKeylessFixture(t, "init-keyless-invalid")
	invalidConfig := libraryFixtureConfig(t)
	invalid, err := bundle.Create(t.Context(), filepath.Join(invalidDir, libraryBundleFileName), bundle.CreateOptions{
		Config:  invalidConfig,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.ErrorIs(t, err, bundle.ErrCreateBundle)
	assert.Nil(t, invalid)
	artifacts, err := filepath.Glob(filepath.Join(invalidDir, "*.tar.zst"))
	require.NoError(t, err)
	assert.Empty(t, artifacts)
}

func keylessVerificationPolicy(t *testing.T, repository, ref string) bundle.VerificationPolicy {
	t.Helper()
	serverURL := strings.TrimRight(os.Getenv("GITHUB_SERVER_URL"), "/")
	if serverURL == "" {
		serverURL = "https://github.com"
	}
	identityRegexp := fmt.Sprintf(
		"^%s/%s/\\.github/workflows/[^@]+@%s$",
		regexp.QuoteMeta(serverURL),
		regexp.QuoteMeta(strings.TrimLeft(repository, "/")),
		regexp.QuoteMeta(ref),
	)
	return bundle.VerificationPolicy{Keyless: &bundle.KeylessVerification{
		CertificateIdentityRegexp: identityRegexp,
		CertificateOIDCIssuer:     "https://token.actions.githubusercontent.com",
	}}
}

func copyKeylessFixture(t *testing.T, name string) string {
	t.Helper()
	source := keylessFixturePath(t, name)
	destination := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.CopyFS(destination, os.DirFS(source)))
	return destination
}

func keylessFixturePath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve keyless fixture path")
	path := filepath.Join(filepath.Dir(file), "..", "test_data", "bundles", "signature-verification", name)
	require.DirExists(t, path)
	return path
}
