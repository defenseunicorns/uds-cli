// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package bundle_test

import (
	"fmt"
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

const k3sLayerTitle = "components/k3s.tar"

func assertNoBundleArchive(t *testing.T, dir string) {
	t.Helper()

	archives, err := filepath.Glob(filepath.Join(dir, "*.tar.zst"))
	require.NoError(t, err)
	assert.Empty(t, archives, "failed bundle creation must not write an archive")
}

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

// TestCreate_InitBundle verifies that the base init bundle
// creates a valid bundle archive with proper OCI layout structure.
func TestCreate_InitBundle(t *testing.T) {
	outPath := testutil.CreateBundleFromTestData(t, "bundles/create/init", runtime.GOARCH)
	assertValidBundleStructure(t, outPath)
}

func TestCreate_SignatureVerification(t *testing.T) {
	t.Run("public key verification succeeds", func(t *testing.T) {
		outPath := testutil.CreateBundleFromTestData(t, filepath.Join("bundles", "signature-verification", "dos-games-public-key"), runtime.GOARCH)
		assertValidBundleStructure(t, outPath)
	})

	t.Run("mismatched public key fails before writing an archive", func(t *testing.T) {
		dir := testutil.CreateBundleFromTestDataExpectError(t, filepath.Join("bundles", "signature-verification", "dos-games-public-key-invalid"), runtime.GOARCH)
		assertNoBundleArchive(t, dir)
	})

	t.Run("keyless verification succeeds", func(t *testing.T) {
		outPath := testutil.CreateBundleFromTestData(t, filepath.Join("bundles", "signature-verification", "init-keyless"), runtime.GOARCH)
		assertValidBundleStructure(t, outPath)
	})

	t.Run("mismatched keyless identity fails before writing an archive", func(t *testing.T) {
		dir := testutil.CreateBundleFromTestDataExpectError(t, filepath.Join("bundles", "signature-verification", "init-keyless-invalid"), runtime.GOARCH)
		assertNoBundleArchive(t, dir)
	})

	t.Run("verification disabled succeeds with an unverified-package warning", func(t *testing.T) {
		outPath, diagnostics := testutil.CreateBundleFromTestDataWithDiagnostics(t, filepath.Join("bundles", "signature-verification", "init-verification-disabled"), runtime.GOARCH)
		assert.Contains(t, diagnostics, "unverified package")
		assertValidBundleStructure(t, outPath)
	})
}

// TestCreate_InitBundle_OptionalComponentIncluded verifies that when k3s is listed
// in optional_components, its layer blob is present in the output bundle.
func TestCreate_InitBundle_OptionalComponentIncluded(t *testing.T) {
	outPath := testutil.CreateBundleFromTestData(t, "bundles/create/init-k3s", runtime.GOARCH)

	allPaths, small := assertValidBundleStructure(t, outPath)

	assert.True(t, bundleContainsLayerTitle(t, allPaths, small, k3sLayerTitle),
		"k3s layer blob should be present when k3s is listed in optional_components")
}

// TestCreate_InitBundle_OptionalComponentExcluded verifies that when
// optional_components is omitted, the k3s layer blob is absent from the bundle.
func TestCreate_InitBundle_OptionalComponentExcluded(t *testing.T) {
	outPath := testutil.CreateBundleFromTestData(t, "bundles/create/init-no-k3s", runtime.GOARCH)

	allPaths, small := assertValidBundleStructure(t, outPath)

	assert.False(t, bundleContainsLayerTitle(t, allPaths, small, k3sLayerTitle),
		"k3s layer blob should be absent when optional_components is omitted")
}

// TestCreate_DefaultsConfig_Applied verifies that when a defaults.uds.hcl exists
// alongside bundle.uds.hcl, its variables are applied during create
func TestCreate_DefaultsConfig_Applied(t *testing.T) {
	srcDir := testutil.TestDataPath("bundles/create/init-with-defaults")

	dir := t.TempDir()
	require.NoError(t, testutil.CopyDir(srcDir, dir))

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
	assert.Equal(t, float64(0), resolved.Variables["b"])
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

// TestCreate_DefaultsConfig_IncludedAsOCILayer verifies that when defaults.uds.hcl exists alongside
// bundle.uds.hcl, it is included as a layer in the bundle definition manifest in the OCI layout.
func TestCreate_DefaultsConfig_IncludedAsOCILayer(t *testing.T) {
	outPath := testutil.CreateBundleFromTestData(t, "bundles/create/init-with-defaults", runtime.GOARCH)

	allPaths, small := assertValidBundleStructure(t, outPath)

	assert.True(t,
		bundleDefinitionContainsLayerTitle(t, allPaths, small, "defaults.uds.hcl"),
		"defaults.uds.hcl should be a layer in the bundle definition manifest",
	)
}

// TestCreate_InitBundle_MultiArch verifies that the init bundle can be created
// for multiple architectures and that each output is named correctly.
func TestCreate_InitBundle_MultiArch(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			outPath := testutil.CreateBundleFromTestData(t, "bundles/create/init", arch)

			expectedSuffix := fmt.Sprintf("uds-bundle-k3d-core-init-%s-0.1.0.tar.zst", arch)
			assert.True(t, strings.HasSuffix(outPath, expectedSuffix),
				"bundle filename should contain arch %q, got: %s", arch, filepath.Base(outPath))

			assertValidBundleStructure(t, outPath)
		})
	}
}

func TestCreate_UDSCoreStandardBundle(t *testing.T) {
	outPath := testutil.CreateBundleFromTestDataCobra(t, "bundles/uds-core/standard", runtime.GOARCH)

	allPaths, small := assertValidBundleStructure(t, outPath)

	assert.True(t,
		bundleDefinitionContainsLayerTitle(t, allPaths, small, "defaults.uds.hcl"),
		"defaults.uds.hcl should be a layer in the bundle definition manifest",
	)
	assert.True(t,
		bundleDefinitionContainsLayerTitle(t, allPaths, small, "values/core_base/0.yaml"),
		"core-base values file should be included in the bundle definition manifest",
	)
	assert.True(t,
		bundleDefinitionContainsLayerTitle(t, allPaths, small, "values/core_identity_authorization/0.yaml"),
		"core-identity-authorization values file should be included in the bundle definition manifest",
	)
	assert.True(t,
		bundleDefinitionContainsLayerTitle(t, allPaths, small, "values/core_logging/0.yaml"),
		"core-logging values file should be included in the bundle definition manifest",
	)
	assert.True(t,
		bundleDefinitionContainsLayerTitle(t, allPaths, small, "values/core_monitoring/0.yaml"),
		"core-monitoring values file should be included in the bundle definition manifest",
	)
	assert.True(t,
		bundleDefinitionContainsLayerTitle(t, allPaths, small, "values/core_runtime_security/0.yaml"),
		"core-runtime-security values file should be included in the bundle definition manifest",
	)
	assert.True(t,
		bundleDefinitionContainsLayerTitle(t, allPaths, small, "values/core_backup_restore/0.yaml"),
		"core-backup-restore values file should be included in the bundle definition manifest",
	)

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
	assert.Contains(t, bundleDefinition, "1.9.0-upstream", "standard should target the released core package tag")
}
