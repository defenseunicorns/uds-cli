// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package bundle_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	bundlecmd "github.com/defenseunicorns/uds-cli/pkg/cmd/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

const k3sLayerTitle = "components/k3s.tar"

// createBundleFromTestData copies the bundle HCL from test_data into a temp dir
// and runs bundle.Create, returning the output path.
func createBundleFromTestData(t *testing.T, testDataRelPath, arch string) string {
	t.Helper()

	srcDir := testDataPath(t, testDataRelPath)
	bundleFile := filepath.Join(srcDir, "bundle.uds.hcl")
	_, err := os.Stat(bundleFile)
	require.NoError(t, err, "bundle.uds.hcl must exist in %s", srcDir)

	// Copy the bundle directory to a temp dir so the output artifact
	// does not pollute the source tree.
	dir := t.TempDir()
	require.NoError(t, copyDir(srcDir, dir))

	streams, _, _, _ := iostreams.NewTestIOStreams()

	resolver := bundlecmd.NewConfigResolver()
	opts := resolver.Defaults()
	opts.Architecture = arch
	global := &bundlepkg.GlobalOptions{LogLevel: opts.LogLevel}
	result, err := bundlepkg.Create(t.Context(), bundlepkg.CreateOptions{
		Config:     &bundlepkg.UDSBundleConfig{Global: global, Options: &opts},
		BundleFile: filepath.Join(dir, "bundle.uds.hcl"),
		Streams:    streams,
	})
	require.NoError(t, err)

	_, err = os.Stat(result.OutputPath)
	require.NoError(t, err, "expected bundle output file to exist")
	return result.OutputPath
}

// copyDir recursively copies the contents of src into dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
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
	outPath := createBundleFromTestData(t, "bundles/create/init", runtime.GOARCH)
	assertValidBundleStructure(t, outPath)
}

// TestCreate_InitBundle_OptionalComponentIncluded verifies that when k3s is listed
// in optional_components, its layer blob is present in the output bundle.
func TestCreate_InitBundle_OptionalComponentIncluded(t *testing.T) {
	outPath := createBundleFromTestData(t, "bundles/create/init-k3s", runtime.GOARCH)

	allPaths, small := assertValidBundleStructure(t, outPath)

	assert.True(t, bundleContainsLayerTitle(t, allPaths, small, k3sLayerTitle),
		"k3s layer blob should be present when k3s is listed in optional_components")
}

// TestCreate_InitBundle_OptionalComponentExcluded verifies that when
// optional_components is omitted, the k3s layer blob is absent from the bundle.
func TestCreate_InitBundle_OptionalComponentExcluded(t *testing.T) {
	outPath := createBundleFromTestData(t, "bundles/create/init-no-k3s", runtime.GOARCH)

	allPaths, small := assertValidBundleStructure(t, outPath)

	assert.False(t, bundleContainsLayerTitle(t, allPaths, small, k3sLayerTitle),
		"k3s layer blob should be absent when optional_components is omitted")
}

// TestCreate_DefaultsConfig_Applied verifies that when a defaults.uds.hcl exists
// alongside bundle.uds.hcl, its variables are applied during create
func TestCreate_DefaultsConfig_Applied(t *testing.T) {
	srcDir := testDataPath(t, "bundles/create/init-with-defaults")

	dir := t.TempDir()
	require.NoError(t, copyDir(srcDir, dir))

	// Exercise cobra wiring: create the bundle via the bundle command
	streams, _, out, _ := iostreams.NewTestIOStreams()

	root := bundlecmd.NewBundleCommand(streams)
	root.SetArgs([]string{"create", dir})

	err := root.Execute()
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "Bundle Name:")

	// Verify defaults variables are resolved through ConfigResolver
	resolver := bundlecmd.NewConfigResolver()
	cmd := bundlecmd.NewBundleCommand(streams)
	// Find the create subcommand to get its flags
	createCmd, _, _ := cmd.Find([]string{"create"})
	createCmd.Flags().String("config", "", "config path")
	resolved, _, err := resolver.Resolve(t.Context(), iostreams.IOStreams{}, bundlecmd.SnapshotFlags(createCmd), dir)
	require.NoError(t, err)

	// Variables from defaults.uds.hcl
	require.NotNil(t, resolved.Variables)
	assert.Equal(t, "a-default-value", resolved.Variables["a"])
	assert.Equal(t, float64(0), resolved.Variables["b"])
	c, ok := resolved.Variables["c"].(bundlepkg.Variables)
	require.True(t, ok)
	assert.Equal(t, true, c["d"])
	assert.Equal(t, false, c["e"])
}

// TestCreate_DefaultsConfig_IncludedAsOCILayer verifies that when defaults.uds.hcl exists alongside
// bundle.uds.hcl, it is included as a layer in the bundle definition manifest in the OCI layout.
func TestCreate_DefaultsConfig_IncludedAsOCILayer(t *testing.T) {
	outPath := createBundleFromTestData(t, "bundles/create/init-with-defaults", runtime.GOARCH)

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
			outPath := createBundleFromTestData(t, "bundles/create/init", arch)

			expectedSuffix := fmt.Sprintf("uds-bundle-k3d-core-init-%s-0.1.0.tar.zst", arch)
			assert.True(t, strings.HasSuffix(outPath, expectedSuffix),
				"bundle filename should contain arch %q, got: %s", arch, filepath.Base(outPath))

			assertValidBundleStructure(t, outPath)
		})
	}
}
