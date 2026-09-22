// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cli

package bundle_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/cli/bundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

// assertValidBundleStructure checks that the bundle archive contains the
// expected OCI layout structure and that bundle.uds.hcl is a layer in the
// bundle definition manifest. Returns the entries for callers that need further checks.
func assertValidBundleStructure(t *testing.T, tarPath string) (allPaths map[string]bool, small map[string][]byte) {
	t.Helper()
	allPaths, small = readBundleEntries(t, tarPath)
	assert.True(t, allPaths["oci/oci-layout"], "bundle should contain oci-layout file")
	assert.True(t, allPaths["oci/index.json"], "bundle should contain index.json")
	assert.True(t, bundleDefinitionContainsLayerTitle(t, allPaths, small, "bundle.uds.hcl"), "bundle.uds.hcl should be a layer in the bundle definition manifest")

	foundBlob := false
	for path := range allPaths {
		if strings.HasPrefix(path, "oci/blobs/sha256/") && path != "oci/blobs/sha256/" {
			foundBlob = true
			break
		}
	}
	assert.True(t, foundBlob, "bundle should contain at least one blob under oci/blobs/sha256/")
	return allPaths, small
}

func TestCreate_VerificationDisabledReportsUnverifiedPackage(t *testing.T) {
	streams, _, _, errOut := iostreams.NewTestIOStreams()
	root := bundle.NewBundleCommand(streams)
	root.SetArgs([]string{"create", "--unsigned", createDefaultsBundleSource(t)})
	require.NoError(t, root.Execute())
	assert.Contains(t, errOut.String(), "unverified package")
}

// TestCreate_DefaultsConfig_Applied verifies that when a defaults.uds.hcl exists
// alongside bundle.uds.hcl, its variables are applied during create
func TestCreate_DefaultsConfig_Applied(t *testing.T) {
	dir := createDefaultsBundleSource(t)

	// Exercise cobra wiring: create the bundle via the bundle command
	streams, _, out, _ := iostreams.NewTestIOStreams()

	root := bundle.NewBundleCommand(streams)
	root.SetArgs([]string{"create", "--unsigned", dir})

	err := root.Execute()
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "Bundle Name:")

	// Verify defaults variables are resolved through ConfigResolver
	resolver := bundle.NewConfigResolver()
	cmd := bundle.NewBundleCommand(streams)
	// Find the create subcommand to get its flags
	createCmd, _, _ := cmd.Find([]string{"create"})
	createCmd.Flags().String("config", "", "config path")
	resolved, _, err := resolver.Resolve(t.Context(), iostreams.IOStreams{}, bundle.SnapshotFlags(createCmd), dir)
	require.NoError(t, err)

	// Variables from defaults.uds.hcl
	require.NotNil(t, resolved.Variables)
	a, ok := resolved.Variables["a"].(string)
	require.Truef(t, ok, "expected variable a to be a string, got %T", resolved.Variables["a"])
	assert.Equal(t, "from-file", strings.TrimSpace(a))
	b, ok := resolved.Variables["b"].(float64)
	require.True(t, ok)
	assert.Zero(t, b)
	c, ok := resolved.Variables["c"].(bundlepkg.Variables)
	require.True(t, ok)
	assert.Equal(t, true, c["d"])
	assert.Equal(t, false, c["e"])

	artifactPath := filepath.Join(dir, "uds-bundle-defaults-test-"+runtime.GOARCH+"-0.1.0.tar.zst")
	_, small := assertValidBundleStructure(t, artifactPath)
	storedDefaults := extractLayerFromBundle(t, small, bundleinternal.BundleDefaultsFileName)
	assert.NotContains(t, string(storedDefaults), "file(")
	assert.Contains(t, string(storedDefaults), "from-file")
	storedBundle := extractLayerFromBundle(t, small, bundleinternal.BundleFileName)
	assert.NotContains(t, string(storedBundle), "file(")
	assert.Contains(t, string(storedBundle), "description from file")
}

func TestCreate_UDSCoreStandardBundle(t *testing.T) {
	outPath := testutil.CreateBundleFromTestDataCobra(t, "bundles/uds-core/standard", runtime.GOARCH)

	allPaths, small := assertValidBundleStructure(t, outPath)
	for _, layer := range []string{
		"defaults.uds.hcl",
		"values/core_base/0.yaml",
		"values/core_identity_authorization/0.yaml",
		"values/core_logging/0.yaml",
		"values/core_monitoring/0.yaml",
		"values/core_runtime_security/0.yaml",
		"values/core_backup_restore/0.yaml",
	} {
		assert.True(t, bundleDefinitionContainsLayerTitle(t, allPaths, small, layer), "%s should be included in the bundle definition manifest", layer)
	}

	bundleDefinition := string(extractLayerFromBundle(t, small, "bundle.uds.hcl"))
	for _, packageID := range []string{
		"package \"uds_k3d_dev\"",
		"package \"init\"",
		"package \"core_base\"",
		"package \"core_identity_authorization\"",
		"package \"core_logging\"",
		"package \"core_monitoring\"",
		"package \"core_runtime_security\"",
		"package \"core_backup_restore\"",
		"package \"core_portal\"",
		"package \"core_metrics_server\"",
	} {
		assert.Contains(t, bundleDefinition, packageID, "standard should preserve upstream package composition")
	}
	assert.NotContains(t, bundleDefinition, "package \"core\"", "standard should not collapse the release back to the monolithic core package")
	for _, component := range []string{"istio-passthrough-gateway", "istio-egress-gateway", "envoy-gateway", "envoy-default-gateway"} {
		assert.Contains(t, bundleDefinition, component, "standard should preserve upstream optional components")
	}
	// renovate: datasource=docker depName=ghcr.io/defenseunicorns/packages/uds/core versioning=docker
	assert.Contains(t, bundleDefinition, "1.13.0-upstream", "standard should target the released core package tag")
}

func createDefaultsBundleSource(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	packageDir := filepath.Join(dir, "pkg")
	require.NoError(t, os.Mkdir(packageDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(packageDir, "zarf.yaml"), []byte(`build:
  signed: true
metadata:
  name: init
  version: 1.0.0
  aggregateChecksum: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(packageDir, "checksums.txt"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "default-a.txt"), []byte("from-file\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "description.txt"), []byte("description from file\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "defaults.uds.hcl"), []byte(`variables = {
  a = file("default-a.txt")
  b = 0
  c = {
    d = true
    e = false
  }
}
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bundle.uds.hcl"), []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name        = "defaults-test"
  description = file("description.txt")
  version     = "0.1.0"
}
package "init" {
  source = "pkg"
  signature_verification { verify = false }
}
`), 0o600))
	return dir
}

func createDefaultsArtifact(t *testing.T) string {
	t.Helper()

	dir := createDefaultsBundleSource(t)
	result, err := bundlepkg.Create(t.Context(), filepath.Join(dir, bundleinternal.BundleFileName), bundlepkg.CreateOptions{
		Config: &bundlepkg.UDSBundleConfig{Options: &bundlepkg.ConfigOptions{
			Architecture: runtime.GOARCH,
			Concurrency:  1,
			TmpDir:       t.TempDir(),
		}},
		Signing: bundlepkg.SigningOptions{Mode: bundlepkg.SigningModeUnsigned},
	})
	require.NoError(t, err)
	return result.OutputPath
}

func createDefaultsBundleSourceWithUDSK3DDev(t *testing.T) string {
	t.Helper()

	dir := createDefaultsBundleSource(t)
	bundleFile := filepath.Join(dir, bundleinternal.BundleFileName)
	contents, err := os.ReadFile(bundleFile)
	require.NoError(t, err)
	contents = append(contents, []byte(`
package "uds_k3d_dev" {
  source = "pkg"
  signature_verification { verify = false }
}
`)...)
	require.NoError(t, os.WriteFile(bundleFile, contents, 0o600)) //nolint:gosec // bundleFile is inside a test-owned temporary directory.
	return dir
}
