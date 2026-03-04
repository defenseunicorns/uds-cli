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

	regOpts := bundlepkg.DefaultRegistryOptions()
	regOpts.Arch = arch
	outPath, err := bundlepkg.Create(t.Context(), bundlepkg.CreateOptions{
		RegistryOptions: regOpts,
		BundleFile:      filepath.Join(dir, "bundle.uds.hcl"),
		Out:             streams.Out,
	})
	require.NoError(t, err)

	_, err = os.Stat(outPath)
	require.NoError(t, err, "expected bundle output file to exist")
	return outPath
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
// expected OCI layout structure and the HCL source file.
func assertValidBundleStructure(t *testing.T, entries map[string]bool) {
	t.Helper()
	assert.True(t, entries["oci/oci-layout"], "bundle should contain oci-layout file")
	assert.True(t, entries["oci/index.json"], "bundle should contain index.json")
	assert.True(t, entries["uds-bundle.hcl"], "bundle should contain uds-bundle.hcl")

	foundBlob := false
	for path := range entries {
		if strings.HasPrefix(path, "oci/blobs/sha256/") && path != "oci/blobs/sha256/" {
			foundBlob = true
			break
		}
	}
	assert.True(t, foundBlob, "bundle should contain at least one blob under oci/blobs/sha256/")
}

// TestCreate_InitBundle verifies that the base init bundle
// creates a valid bundle archive with proper OCI layout structure.
func TestCreate_InitBundle(t *testing.T) {
	outPath := createBundleFromTestData(t, "bundles/create/init", runtime.GOARCH)

	entries, _ := readBundleEntries(t, outPath)
	assertValidBundleStructure(t, entries)
}

// TestCreate_InitBundle_OptionalComponentIncluded verifies that when k3s is listed
// in optional_components, its layer blob is present in the output bundle.
func TestCreate_InitBundle_OptionalComponentIncluded(t *testing.T) {
	outPath := createBundleFromTestData(t, "bundles/create/init-k3s", runtime.GOARCH)

	entries, _ := readBundleEntries(t, outPath)
	assertValidBundleStructure(t, entries)

	assert.True(t, bundleContainsLayerTitle(t, outPath, k3sLayerTitle),
		"k3s layer blob should be present when k3s is listed in optional_components")
}

// TestCreate_InitBundle_OptionalComponentExcluded verifies that when
// optional_components is omitted, the k3s layer blob is absent from the bundle.
func TestCreate_InitBundle_OptionalComponentExcluded(t *testing.T) {
	outPath := createBundleFromTestData(t, "bundles/create/init-no-k3s", runtime.GOARCH)

	entries, _ := readBundleEntries(t, outPath)
	assertValidBundleStructure(t, entries)

	assert.False(t, bundleContainsLayerTitle(t, outPath, k3sLayerTitle),
		"k3s layer blob should be absent when optional_components is omitted")
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

			entries, _ := readBundleEntries(t, outPath)
			assertValidBundleStructure(t, entries)
		})
	}
}
