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

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveLocalArtifactSource(t *testing.T) {
	artifact := createRemoveArtifact(t)
	assertArtifactReferenceReachesRemove(t, artifact, removeArtifactConfig(t))
}
func TestRemoveOCIArtifactSource(t *testing.T) {
	artifact := createRemoveArtifact(t)
	registryHost := testutil.StartLocalRegistry(t)
	config := removeArtifactConfig(t)
	config.Options.PlainHTTP = true
	ref := fmt.Sprintf("%s/test/remove:v1.0.0", registryHost)
	pushed, err := bundle.Push(t.Context(), artifact, ref, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	require.NotNil(t, pushed)
	assertArtifactReferenceReachesRemove(t, ref, config)
}
func assertArtifactReferenceReachesRemove(t *testing.T, ref string, config *bundle.UDSBundleConfig) {
	t.Helper()
	// An invalid package selection stops before cluster access. Reaching that
	// validation proves Remove resolved and parsed the artifact reference first.
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
func TestRemoveLocalArtifactSource_UnavailableReference(t *testing.T) {
	source, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, filepath.Join(t.TempDir(), "missing.tar.zst"), t.TempDir(), runtime.GOARCH)
	require.ErrorContains(t, err, "extracting bundle artifact")
	assert.Nil(t, source)
}
func createRemoveArtifact(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	rootFS, err := os.OpenRoot(root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rootFS.Close()) })
	require.NoError(t, rootFS.Mkdir("pkg", 0o755))
	require.NoError(t, rootFS.WriteFile("pkg/zarf.yaml", []byte("build:\n  signed: true\nmetadata:\n  name: test\n  version: 1.0.0\n  aggregateChecksum: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\n"), 0o644))
	require.NoError(t, rootFS.WriteFile("pkg/checksums.txt", nil, 0o644))
	bundleFile := filepath.Join(root, bundleinternal.BundleFileName)
	require.NoError(t, rootFS.WriteFile(bundleinternal.BundleFileName, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "remove-integration"
  version = "1.0.0"
}
package "pkg" {
  source = "pkg"
  signature_verification { verify = false }
}
`), 0o644))
	result, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
		Config:  removeArtifactConfig(t),
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
		Streams: iostreams.IOStreams{},
	})
	require.NoError(t, err)
	return result.OutputPath
}
func removeArtifactConfig(t *testing.T) *bundle.UDSBundleConfig {
	t.Helper()
	return &bundle.UDSBundleConfig{
		Options: &bundle.ConfigOptions{
			Architecture: runtime.GOARCH,
			Concurrency:  10,
			TmpDir:       t.TempDir(),
		},
	}
}
