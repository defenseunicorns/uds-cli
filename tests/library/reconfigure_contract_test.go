// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	fixtureartifact "github.com/defenseunicorns/uds-cli/tests/fixtures/artifact"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconfigureLocalPublicContract(t *testing.T) {
	fixture := createLibraryBundle(t)
	originalBytes, err := os.ReadFile(fixture.ArtifactPath)
	require.NoError(t, err)
	original, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: fixture.ArtifactPath, Config: fixture.Config})
	require.NoError(t, err)
	privateKey, publicKey := libraryKeyPair(t)
	defaults := writeLibraryDefaults(t, `variables = { fixture = "reconfigured" }`)
	outputDir := t.TempDir()

	reconfigured, err := bundle.Reconfigure(t.Context(), fixture.ArtifactPath, defaults, bundle.ReconfigureOptions{
		Suffix:                    "-reconfigured",
		OutputDir:                 outputDir,
		Config:                    fixture.Config,
		Signing:                   bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
		SkipSignatureVerification: true,
	})
	require.NoError(t, err)
	require.NotNil(t, reconfigured)
	require.NotEmpty(t, reconfigured.OutputPath)
	assert.Empty(t, reconfigured.OCIReference)
	assert.Equal(t, outputDir, filepath.Dir(reconfigured.OutputPath))
	require.FileExists(t, reconfigured.OutputPath)
	assert.NotEqual(t, fixture.ArtifactPath, reconfigured.OutputPath)

	policy := bundle.VerificationPolicy{PublicKey: publicKey}
	require.NoError(t, bundle.Verify(t.Context(), bundle.VerifyOptions{
		Source: reconfigured.OutputPath,
		Policy: policy,
		Config: fixture.Config,
		TmpDir: t.TempDir(),
	}))
	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{
		Source: reconfigured.OutputPath,
		Config: &bundle.UDSBundleConfig{
			Options:               fixture.Config.Options,
			SignatureVerification: &policy,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "library-fixture-reconfigured", inspected.Name)
	assert.Equal(t, original.ArtifactDigest, inspected.ReconfiguredFrom)
	assert.Equal(t, bundle.BundleSignatureStatusVerified, inspected.BundleSignature.Status)
	assert.Equal(t, []string{"base", "app"}, []string{inspected.Packages[0].Name, inspected.Packages[1].Name})
	assert.Contains(t, libraryPreparedDefaults(t, reconfigured.OutputPath, fixture.Config), "reconfigured")

	originalBytesAfter, err := os.ReadFile(fixture.ArtifactPath)
	require.NoError(t, err)
	assert.Equal(t, originalBytes, originalBytesAfter)
	original, err = bundle.Inspect(t.Context(), bundle.InspectOptions{Source: fixture.ArtifactPath, Config: fixture.Config})
	require.NoError(t, err)
	assert.Equal(t, "library-fixture", original.Name)
	assert.Empty(t, original.ReconfiguredFrom)
	assert.Contains(t, libraryPreparedDefaults(t, fixture.ArtifactPath, fixture.Config), "default")
}

func TestReconfigureOCIPublicContract(t *testing.T) {
	fixture := createLibraryBundle(t)
	config := libraryRegistryConfig(fixture.Config)
	ref := fmt.Sprintf("%s/library/reconfigure:v1.0.0", startLibraryRegistry(t))
	pushed, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	original, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: pushed.OCIReference, Config: config})
	require.NoError(t, err)
	originalDigest := original.ArtifactDigest
	defaults := writeLibraryDefaults(t, `variables = { fixture = "oci-reconfigured" }`)

	reconfigured, err := bundle.Reconfigure(t.Context(), pushed.OCIReference, defaults, bundle.ReconfigureOptions{
		Suffix:                    "-reconfigured",
		Config:                    config,
		Signing:                   bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
		SkipSignatureVerification: true,
	})
	require.NoError(t, err)
	require.NotNil(t, reconfigured)
	assert.Empty(t, reconfigured.OutputPath)
	assert.Equal(t, "oci://"+ref+"-reconfigured", reconfigured.OCIReference)

	pulled, err := bundle.Pull(t.Context(), reconfigured.OCIReference, t.TempDir(), bundle.PullOptions{
		Config:                    config,
		SkipSignatureVerification: true,
	})
	require.NoError(t, err)
	require.FileExists(t, pulled.OutputPath)
	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{
		Source:                    reconfigured.OCIReference,
		Config:                    config,
		SkipSignatureVerification: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "library-fixture-reconfigured", inspected.Name)
	assert.Equal(t, originalDigest, inspected.ReconfiguredFrom)
	assert.Contains(t, libraryPreparedDefaults(t, pulled.OutputPath, config), "oci-reconfigured")

	originalAfter, err := bundle.Inspect(t.Context(), bundle.InspectOptions{
		Source:                    pushed.OCIReference,
		Config:                    config,
		SkipSignatureVerification: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "library-fixture", originalAfter.Name)
	assert.Empty(t, originalAfter.ReconfiguredFrom)
	assert.Equal(t, originalDigest, originalAfter.ArtifactDigest)
}

func TestReconfigureVerifiesSourcePolicy(t *testing.T) {
	fixture := createLibraryBundle(t)
	privateKey, publicKey := libraryKeyPair(t)
	require.NoError(t, bundle.Sign(t.Context(), bundle.SignOptions{
		Source:  fixture.ArtifactPath,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
		Config:  fixture.Config,
		TmpDir:  t.TempDir(),
	}))
	defaults := writeLibraryDefaults(t, `variables = { fixture = "verified" }`)
	policy := bundle.VerificationPolicy{PublicKey: publicKey}
	result, err := bundle.Reconfigure(t.Context(), fixture.ArtifactPath, defaults, bundle.ReconfigureOptions{
		Suffix:       "-verified",
		OutputDir:    t.TempDir(),
		Config:       fixture.Config,
		Signing:      bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
		Verification: policy,
	})
	require.NoError(t, err)
	require.FileExists(t, result.OutputPath)

	_, wrongPublicKey := libraryKeyPair(t)
	_, err = bundle.Reconfigure(t.Context(), fixture.ArtifactPath, defaults, bundle.ReconfigureOptions{
		Suffix:       "-wrong-key",
		OutputDir:    t.TempDir(),
		Config:       fixture.Config,
		Signing:      bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
		Verification: bundle.VerificationPolicy{PublicKey: wrongPublicKey},
	})
	require.ErrorIs(t, err, bundle.ErrReconfigureBundle)
}

func TestReconfigureAddsDefaultsAndPreservesPackageManifests(t *testing.T) {
	fixture := createLibraryBundleWithoutDefaults(t)
	original, err := fixtureartifact.Read(t.Context(), fixture.ArtifactPath)
	require.NoError(t, err)
	originalPackages, err := original.PackageManifestDigests()
	require.NoError(t, err)
	defaults := writeLibraryDefaults(t, `variables = { fixture = "added" }`)

	reconfigured, err := bundle.Reconfigure(t.Context(), fixture.ArtifactPath, defaults, bundle.ReconfigureOptions{
		Suffix:                    "-added",
		OutputDir:                 t.TempDir(),
		Config:                    fixture.Config,
		Signing:                   bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
		SkipSignatureVerification: true,
	})
	require.NoError(t, err)
	require.NotNil(t, reconfigured)
	assert.Contains(t, filepath.Base(reconfigured.OutputPath), "-added-")
	reconfiguredEntries, err := fixtureartifact.Read(t.Context(), reconfigured.OutputPath)
	require.NoError(t, err)
	hasDefaults, err := reconfiguredEntries.HasLayerInArtifact("defaults.uds.hcl", fixtureartifact.BundleDefinitionMediaType)
	require.NoError(t, err)
	assert.True(t, hasDefaults)
	reconfiguredPackages, err := reconfiguredEntries.PackageManifestDigests()
	require.NoError(t, err)
	assert.Equal(t, originalPackages, reconfiguredPackages)
	assert.Contains(t, libraryPreparedDefaults(t, reconfigured.OutputPath, fixture.Config), "added")
}

func writeLibraryDefaults(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "defaults.uds.hcl")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

func libraryPreparedDefaults(t *testing.T, artifactPath string, config *bundle.UDSBundleConfig) string {
	t.Helper()
	source, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, artifactPath, t.TempDir(), runtime.GOARCH)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, source.Close()) })
	require.NotEmpty(t, source.DefaultsPath)
	contents, err := os.ReadFile(source.DefaultsPath)
	require.NoError(t, err)
	return strings.TrimSpace(string(contents))
}
