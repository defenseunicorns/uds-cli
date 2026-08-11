// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package bundle_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	bundlecmd "github.com/defenseunicorns/uds-cli/internal/cli/bundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

func TestReconfigure_LocalTarball(t *testing.T) {
	outPath := testutil.CreateBundleFromTestData(t, "bundles/create/init-with-defaults", runtime.GOARCH)
	allPaths, small := assertValidBundleStructure(t, outPath)

	// Original has defaults.
	assert.True(t, bundleDefinitionContainsLayerTitle(t, allPaths, small, "defaults.uds.hcl"))

	// Write new defaults.
	newDefaultsPath := filepath.Join(t.TempDir(), "new-defaults.uds.hcl")
	require.NoError(t, os.WriteFile(newDefaultsPath, []byte(`variables = {
  domain = "production.example.com"
  replicas = 3
}
`), 0o600))

	outDir := t.TempDir()
	streams, _, out, _ := iostreams.NewTestIOStreams()
	root := bundlecmd.NewBundleCommand(streams)
	root.SetArgs([]string{
		"reconfigure", outPath,
		"--defaults", newDefaultsPath,
		"--suffix", "-prod",
		"--output-dir", outDir,
	})
	require.NoError(t, root.Execute())

	output := out.String()
	assert.Contains(t, output, "Output Path:")

	// Find the reconfigured tarball.
	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	var reconfiguredPath string
	for _, e := range entries {
		if strings.Contains(e.Name(), "-prod-") && strings.HasSuffix(e.Name(), ".tar.zst") {
			reconfiguredPath = filepath.Join(outDir, e.Name())
		}
	}
	require.NotEmpty(t, reconfiguredPath, "reconfigured tarball should exist in output dir")

	// Validate the reconfigured bundle structure.
	reconfigPaths, reconfigSmall := assertValidBundleStructure(t, reconfiguredPath)
	assert.True(t, bundleDefinitionContainsLayerTitle(t, reconfigPaths, reconfigSmall, "defaults.uds.hcl"))
	assert.True(t, bundleDefinitionContainsLayerTitle(t, reconfigPaths, reconfigSmall, "bundle.uds.hcl"))

	// Verify new defaults has the expected variables, not the old ones.
	defaultsContent := extractLayerFromBundle(t, reconfigSmall, "defaults.uds.hcl")
	defaultsTmpPath := filepath.Join(t.TempDir(), "extracted-defaults.uds.hcl")
	require.NoError(t, os.WriteFile(defaultsTmpPath, defaultsContent, 0o600))
	vars, err := bundlepkg.ParseDefaults(t.Context(), defaultsTmpPath)
	require.NoError(t, err)
	assert.Equal(t, "production.example.com", vars["domain"])
	assert.InDelta(t, float64(3), vars["replicas"], 0.001)
	_, hasOldKey := vars["a"]
	assert.False(t, hasOldKey, "old default variable 'a' should not be present")

	// Verify the bundle name was updated to include the suffix.
	hclContent := extractLayerFromBundle(t, reconfigSmall, "bundle.uds.hcl")
	bundle, err := bundlepkg.NewHCLParser("", iostreams.IOStreams{}).ParseBundleBytes(t.Context(), hclContent)
	require.NoError(t, err)
	assert.Equal(t, "defaults-test-prod", bundle.Metadata.Name)

	// Verify provenance annotation on the manifest.
	reconfiguredFrom := assertHasReconfiguredAnnotation(t, reconfigSmall)

	// Verify inspect exposes the stored provenance annotation.
	inspectStreams, _, inspectOut, _ := iostreams.NewTestIOStreams()
	inspectRoot := bundlecmd.NewBundleCommand(inspectStreams)
	inspectRoot.SetArgs([]string{"inspect", reconfiguredPath, "--output", "json"})
	require.NoError(t, inspectRoot.Execute())
	var inspectResult bundlepkg.InspectResult
	require.NoError(t, json.Unmarshal(inspectOut.Bytes(), &inspectResult))
	assert.Equal(t, reconfiguredFrom, inspectResult.ReconfiguredFrom)
	require.Len(t, inspectResult.Packages, 1)
	require.NotNil(t, inspectResult.Packages[0].Signature)
	assert.Equal(t, bundlepkg.PackageSigningStatusSigned, inspectResult.Packages[0].Signature.Signed)
}

func TestReconfigure_CustomSuffix(t *testing.T) {
	outPath := testutil.CreateBundleFromTestData(t, "bundles/create/init-with-defaults", runtime.GOARCH)

	newDefaultsPath := filepath.Join(t.TempDir(), "defaults.uds.hcl")
	require.NoError(t, os.WriteFile(newDefaultsPath, []byte(`variables = { env = "staging" }`), 0o600))

	outDir := t.TempDir()
	streams, _, _, _ := iostreams.NewTestIOStreams()
	root := bundlecmd.NewBundleCommand(streams)
	root.SetArgs([]string{
		"reconfigure", outPath,
		"--defaults", newDefaultsPath,
		"--suffix", "-il5",
		"--output-dir", outDir,
	})
	require.NoError(t, root.Execute())

	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	found := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "-il5-") && strings.HasSuffix(e.Name(), ".tar.zst") {
			found = true
		}
	}
	assert.True(t, found, "output should have -il5 suffix")
}

func TestReconfigure_InsertsDefaultsWhenOriginalHadNone(t *testing.T) {
	// init bundle has no defaults.uds.hcl.
	outPath := testutil.CreateBundleFromTestData(t, "bundles/create/init", runtime.GOARCH)

	newDefaultsPath := filepath.Join(t.TempDir(), "defaults.uds.hcl")
	require.NoError(t, os.WriteFile(newDefaultsPath, []byte(`variables = { inserted = true }`), 0o600))

	outDir := t.TempDir()
	streams, _, _, _ := iostreams.NewTestIOStreams()
	root := bundlecmd.NewBundleCommand(streams)
	root.SetArgs([]string{
		"reconfigure", outPath,
		"--defaults", newDefaultsPath,
		"--suffix", "-reconfigured",
		"--output-dir", outDir,
	})
	require.NoError(t, root.Execute())

	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	var reconfiguredPath string
	for _, e := range entries {
		if strings.Contains(e.Name(), "-reconfigured-") && strings.HasSuffix(e.Name(), ".tar.zst") {
			reconfiguredPath = filepath.Join(outDir, e.Name())
		}
	}
	require.NotEmpty(t, reconfiguredPath)

	reconfigPaths, reconfigSmall := assertValidBundleStructure(t, reconfiguredPath)
	assert.True(t, bundleDefinitionContainsLayerTitle(t, reconfigPaths, reconfigSmall, "defaults.uds.hcl"),
		"defaults.uds.hcl should be inserted when original had none")
}

func TestReconfigure_OCI(t *testing.T) {
	hostPort := startLocalRegistry(t)

	// Create a bundle and push it to the local registry.
	outPath := testutil.CreateBundleFromTestData(t, "bundles/create/init-with-defaults", runtime.GOARCH)

	pushRef := hostPort + "/test/reconfigure-oci:v1.0.0"
	streams, _, _, _ := iostreams.NewTestIOStreams()
	root := bundlecmd.NewBundleCommand(streams)
	root.SetArgs([]string{"push", outPath, pushRef, "--plain-http"})
	require.NoError(t, root.Execute())

	// Reconfigure from OCI source.
	newDefaultsPath := filepath.Join(t.TempDir(), "defaults.uds.hcl")
	require.NoError(t, os.WriteFile(newDefaultsPath, []byte(`variables = {
  domain = "oci-test.example.com"
}
`), 0o600))

	streams2, _, out2, _ := iostreams.NewTestIOStreams()
	root2 := bundlecmd.NewBundleCommand(streams2)
	root2.SetArgs([]string{
		"reconfigure", "oci://" + pushRef,
		"--defaults", newDefaultsPath,
		"--suffix", "-oci-test",
		"--plain-http",
	})
	require.NoError(t, root2.Execute())

	output := out2.String()
	assert.Contains(t, output, "OCI Reference:")
	assert.Contains(t, output, "v1.0.0-oci-test")

	// Pull the reconfigured bundle and verify its contents.
	pullRef := hostPort + "/test/reconfigure-oci:v1.0.0-oci-test"
	pullDir := t.TempDir()
	streams3, _, _, _ := iostreams.NewTestIOStreams()
	root3 := bundlecmd.NewBundleCommand(streams3)
	root3.SetArgs([]string{"pull", pullRef, "--output-dir", pullDir, "--plain-http"})
	require.NoError(t, root3.Execute())

	// Find the pulled tarball.
	entries, err := os.ReadDir(pullDir)
	require.NoError(t, err)
	var pulledPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tar.zst") {
			pulledPath = filepath.Join(pullDir, e.Name())
		}
	}
	require.NotEmpty(t, pulledPath, "pulled tarball should exist")

	// Verify the reconfigured bundle structure and content.
	reconfigPaths, reconfigSmall := assertValidBundleStructure(t, pulledPath)
	assert.True(t, bundleDefinitionContainsLayerTitle(t, reconfigPaths, reconfigSmall, "defaults.uds.hcl"))

	// Verify new defaults content.
	defaultsContent := extractLayerFromBundle(t, reconfigSmall, "defaults.uds.hcl")
	defaultsTmpPath := filepath.Join(t.TempDir(), "extracted-defaults.uds.hcl")
	require.NoError(t, os.WriteFile(defaultsTmpPath, defaultsContent, 0o600))
	vars, err := bundlepkg.ParseDefaults(t.Context(), defaultsTmpPath)
	require.NoError(t, err)
	assert.Equal(t, "oci-test.example.com", vars["domain"])
	_, hasOldKey := vars["a"]
	assert.False(t, hasOldKey, "old default variable 'a' should not be present")

	// Verify the bundle name was updated.
	hclContent := extractLayerFromBundle(t, reconfigSmall, "bundle.uds.hcl")
	bundle, err := bundlepkg.NewHCLParser("", iostreams.IOStreams{}).ParseBundleBytes(t.Context(), hclContent)
	require.NoError(t, err)
	assert.Equal(t, "defaults-test-oci-test", bundle.Metadata.Name)

	// Verify provenance annotation.
	assertHasReconfiguredAnnotation(t, reconfigSmall)

	// Verify original tag is still intact — reconfigure should not mutate it.
	origPullDir := t.TempDir()
	streams4, _, _, _ := iostreams.NewTestIOStreams()
	root4 := bundlecmd.NewBundleCommand(streams4)
	root4.SetArgs([]string{"pull", pushRef, "--output-dir", origPullDir, "--plain-http"})
	require.NoError(t, root4.Execute(), "original tag should still be pullable after reconfigure")

	origEntries, err := os.ReadDir(origPullDir)
	require.NoError(t, err)
	var origPulledPath string
	for _, e := range origEntries {
		if strings.HasSuffix(e.Name(), ".tar.zst") {
			origPulledPath = filepath.Join(origPullDir, e.Name())
		}
	}
	require.NotEmpty(t, origPulledPath)

	// Original should still have the old defaults, not the new ones.
	_, origSmall := readBundleEntries(t, origPulledPath)
	origDefaults := extractLayerFromBundle(t, origSmall, "defaults.uds.hcl")
	origDefaultsTmp := filepath.Join(t.TempDir(), "orig-defaults.uds.hcl")
	require.NoError(t, os.WriteFile(origDefaultsTmp, origDefaults, 0o600))
	origVars, err := bundlepkg.ParseDefaults(t.Context(), origDefaultsTmp)
	require.NoError(t, err)
	assert.Equal(t, "from-file", origVars["a"], "original bundle should still have old defaults")

	// Verify package manifests are identical between original and reconfigured.
	// Only the bundle definition entry should differ.
	type indexEntry struct {
		Digest       string `json:"digest"`
		ArtifactType string `json:"artifactType"`
	}
	type ociIdx struct {
		Manifests []indexEntry `json:"manifests"`
	}
	var origIdx, reconfIdx ociIdx
	require.NoError(t, json.Unmarshal(origSmall["oci/index.json"], &origIdx))
	require.NoError(t, json.Unmarshal(reconfigSmall["oci/index.json"], &reconfIdx))

	origPkgDigests := map[string]bool{}
	for _, m := range origIdx.Manifests {
		if m.ArtifactType != bundlepkg.MediaTypeBundleDefinition {
			origPkgDigests[m.Digest] = true
		}
	}
	reconfPkgDigests := map[string]bool{}
	for _, m := range reconfIdx.Manifests {
		if m.ArtifactType != bundlepkg.MediaTypeBundleDefinition {
			reconfPkgDigests[m.Digest] = true
		}
	}
	assert.Equal(t, origPkgDigests, reconfPkgDigests,
		"package manifest digests should be identical — reconfigure should only change the bundle definition")
}
