// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveLocalArtifactSource(t *testing.T) {
	artifact := createLibraryArtifact(t)
	assertArtifactReferenceReachesRemove(t, artifact, removeArtifactConfig(t))
}

func TestRemoveArtifactUsesProvidedBundleDefinition(t *testing.T) {
	artifactPath := createLibraryArtifact(t)
	provided := &spec.UDSBundle{
		Metadata: spec.Metadata{Name: "provided"},
		Packages: []spec.Package{{Name: "provided", Source: "provided"}},
	}

	result, err := bundle.Remove(t.Context(), &bundle.DeploySource{BundlePath: artifactPath, Bundle: provided}, bundle.RemoveOptions{
		Config:                    removeArtifactConfig(t),
		Packages:                  []string{"provided"},
		SkipSignatureVerification: true,
		Force:                     true,
	})
	require.ErrorContains(t, err, `package "provided" with source "provided" was not found in bundle index`)
	assert.Nil(t, result)
}

func TestRemoveOCIArtifactSource(t *testing.T) {
	artifact := createLibraryArtifact(t)
	registryHost := startLibraryRegistry(t)
	config := removeArtifactConfig(t)
	config.Options.PlainHTTP = true
	ref := fmt.Sprintf("%s/test/remove:v1.0.0", registryHost)
	pushed, err := bundle.Push(t.Context(), artifact, ref, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	require.NotNil(t, pushed)
	assertArtifactReferenceReachesRemove(t, ref, config)
}

func TestRemoveOCIArtifactSourceWithTarZstSuffix(t *testing.T) {
	artifact := createLibraryArtifact(t)
	registryHost := startLibraryRegistry(t)
	config := removeArtifactConfig(t)
	config.Options.PlainHTTP = true
	ref := fmt.Sprintf("%s/test/remove:v1.tar.zst", registryHost)
	pushed, err := bundle.Push(t.Context(), artifact, ref, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	require.NotNil(t, pushed)

	assertArtifactReferenceReachesRemove(t, ref, config)
}
func assertArtifactReferenceReachesRemove(t *testing.T, ref string, config *bundle.UDSBundleConfig) {
	t.Helper()
	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{
		Source:                    ref,
		Config:                    config,
		SkipSignatureVerification: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, inspected.ArtifactDigest)

	// An invalid package selection stops before cluster access. Reaching that
	// validation proves Remove reloaded the same artifact that was inspected.
	result, err := bundle.Remove(t.Context(), &bundle.DeploySource{BundlePath: ref}, bundle.RemoveOptions{
		Config:                    config,
		Packages:                  []string{"not-in-bundle"},
		SkipSignatureVerification: true,
		Force:                     true,
	})
	require.ErrorContains(t, err, "unknown packages")
	assert.Nil(t, result)
	assert.NotContains(t, err.Error(), "failed to parse bundle")
}

func TestRemoveHCLSourceWithoutArtifactDigest(t *testing.T) {
	root := t.TempDir()
	bundlePath := filepath.Join(root, libraryBundleFileName)
	require.NoError(t, os.WriteFile(bundlePath, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "remove-hcl"
  version = "1.0.0"
}
package "pkg" {
  source = "pkg"
  signature_verification { verify = false }
}
`), 0o600))

	result, err := bundle.Remove(t.Context(), &bundle.DeploySource{BundlePath: bundlePath}, bundle.RemoveOptions{
		Config:   removeArtifactConfig(t),
		Packages: []string{"not-in-bundle"},
		Force:    true,
	})
	require.ErrorContains(t, err, "unknown packages")
	assert.Nil(t, result)
	assert.NotContains(t, err.Error(), "artifact identity")
}

func TestRemoveLocalArtifactSource_UnavailableReference(t *testing.T) {
	source, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, filepath.Join(t.TempDir(), "missing.tar.zst"), t.TempDir(), runtime.GOARCH)
	require.ErrorContains(t, err, "extracting bundle artifact")
	assert.Nil(t, source)
}
func removeArtifactConfig(t *testing.T) *bundle.UDSBundleConfig {
	t.Helper()
	return libraryFixtureConfig(t)
}
