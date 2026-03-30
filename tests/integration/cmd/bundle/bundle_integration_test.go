// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package bundle_test

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestInspectCommand_Integration(t *testing.T) {
	bundlePath := testDataPath(t, "bundles/spec-compliant/bundle.uds.hcl")

	streams, _, out, _ := iostreams.NewTestIOStreams()

	root := bundle.NewBundleCommand(streams)
	root.SetArgs([]string{"inspect", bundlePath})

	err := root.Execute()
	require.NoError(t, err)

	output := out.String()

	// Verify BUNDLE METADATA section
	assert.Contains(t, output, "BUNDLE METADATA")
	assert.Contains(t, output, "Name:        uds-core-example")
	assert.Contains(t, output, "Version:     0.1.0")

	// Verify PACKAGES section
	assert.Contains(t, output, "PACKAGES (3)")

	// Verify locals were fully resolved
	assert.NotContains(t, output, "${local.")

	// Verify resolved source URLs contain the expected OCI prefix (version is managed by Renovate)
	assert.Contains(t, output, "oci://ghcr.io/defenseunicorns/packages/uds/core-base:")
	assert.NotContains(t, output, "${local.version}")

	// Verify depends_on and valuesFiles
	assert.Contains(t, output, "DependsOn: core_base")
	assert.Contains(t, output, "DependsOn: core_base, core_logging")
	assert.Contains(t, output, "Value Files: values/loki.yaml, values/vector.yaml")
	assert.Contains(t, output, "Value Files: values/monitoring.yaml")
	assert.Contains(t, output, "Namespace: monitoring")
}

// Note: Error cases (OCI reference, file not found) are tested in unit tests
// because CheckErr calls os.Exit(1) which would terminate the test process.

func TestDeployCommand_Integration(t *testing.T) {
	bundlePath := testDataPath(t, "bundles/deploy/init")

	streams, in, out, _ := iostreams.NewTestIOStreams()
	// Simulate user declining the deployment via --prompt
	in.WriteString("n\n")

	// Use root command because --prompt is a root-level persistent flag
	root := cmd.NewRootCommand(streams)
	root.SetArgs([]string{"bundle", "deploy", bundlePath, "--prompt"})

	err := root.Execute()
	require.NoError(t, err)

	output := out.String()

	// Verify bundle metadata is displayed
	assert.Contains(t, output, "BUNDLE METADATA")
	assert.Contains(t, output, "Name:        k3d-core-init")

	// Verify packages are displayed
	assert.Contains(t, output, "PACKAGES (2)")
	assert.Contains(t, output, "uds_k3d_dev")
	assert.Contains(t, output, "init")

	// Verify confirmation prompt was shown (requires --prompt)
	assert.Contains(t, output, "Deploy this bundle?")
	assert.Contains(t, output, "Deployment cancelled")
}

func TestDeployCommand_WithBundleFile_Integration(t *testing.T) {
	bundlePath := testDataPath(t, "bundles/deploy/init/bundle.uds.hcl")

	streams, in, out, _ := iostreams.NewTestIOStreams()
	// Simulate user declining the deployment via --prompt
	in.WriteString("n\n")

	// Use root command because --prompt is a root-level persistent flag
	root := cmd.NewRootCommand(streams)
	root.SetArgs([]string{"bundle", "deploy", bundlePath, "--prompt"})

	err := root.Execute()
	require.NoError(t, err)

	output := out.String()

	// Verify bundle metadata is displayed
	assert.Contains(t, output, "Name:        k3d-core-init")
	// Verify confirmation prompt was shown (requires --prompt)
	assert.Contains(t, output, "Deploy this bundle?")
}

func TestPullCommand_Integration(t *testing.T) {
	// Push a bundle programmatically so we can test the pull cobra wiring.
	bundlePath := createBundleFromTestData(t, "bundles/create/init", runtime.GOARCH)
	registryHost := startLocalRegistry(t)
	ref := fmt.Sprintf("%s/test/k3d-core-init:v0.1.0", registryHost)

	err := bundlepkg.Push(t.Context(), bundlepkg.PushOptions{
		BundleTarball:   bundlePath,
		OCIReference:    ref,
		TmpDir:          t.TempDir(),
		RegistryOptions: bundlepkg.RegistryOptions{PlainHTTP: true},
	})
	require.NoError(t, err)

	outDir := t.TempDir()
	streams, _, out, _ := iostreams.NewTestIOStreams()
	root := bundle.NewBundleCommand(streams)
	root.SetArgs([]string{"pull", ref, "--output-dir", outDir, "--plain-http"})

	err = root.Execute()
	require.NoError(t, err)

	assert.Contains(t, out.String(), "Bundle pulled successfully")
}

func TestPushCommand_Integration(t *testing.T) {
	bundlePath := createBundleFromTestData(t, "bundles/create/init", runtime.GOARCH)
	registryHost := startLocalRegistry(t)
	ref := fmt.Sprintf("%s/test/k3d-core-init:v0.1.0", registryHost)

	streams, _, out, _ := iostreams.NewTestIOStreams()
	root := bundle.NewBundleCommand(streams)
	root.SetArgs([]string{"push", bundlePath, ref, "--plain-http"})

	err := root.Execute()
	require.NoError(t, err)

	assert.Contains(t, out.String(), "Bundle pushed successfully")
}

// TestPushPull_RoundTrip verifies that a bundle produced by Create can be pushed
// to a local OCI registry and pulled back, and that the pulled tarball contains
// exactly the same set of blob digests as the original.
func TestPushPull_RoundTrip(t *testing.T) {
	arch := runtime.GOARCH
	registryHost := startLocalRegistry(t)
	ref := fmt.Sprintf("%s/test/k3d-core-init:v0.1.0", registryHost)

	// create the bundle from test data.
	originalPath := createBundleFromTestData(t, "bundles/create/init", arch)
	assertValidBundleStructure(t, originalPath)

	// push the bundle to the local registry.
	err := bundlepkg.Push(t.Context(), bundlepkg.PushOptions{
		BundleTarball: originalPath,
		OCIReference:  ref,
		TmpDir:        t.TempDir(),
		RegistryOptions: bundlepkg.RegistryOptions{
			PlainHTTP: true,
		},
	})
	require.NoError(t, err, "Push should succeed against local registry")

	// pull the bundle from the local registry.
	outDir := t.TempDir()
	pulledPath, err := bundlepkg.Pull(t.Context(), bundlepkg.PullOptions{
		OCIReference: ref,
		OutputDir:    outDir,
		TmpDir:       t.TempDir(),
		RegistryOptions: bundlepkg.RegistryOptions{
			PlainHTTP: true,
			Arch:      arch,
		},
	})
	require.NoError(t, err, "Pull should succeed against local registry")

	assertValidBundleStructure(t, pulledPath)
	assertBundleTarballsEqual(t, originalPath, pulledPath)
}

func TestRemoveCommand_Integration(t *testing.T) {
	streams, _, out, _ := iostreams.NewTestIOStreams()

	root := bundle.NewBundleCommand(streams)
	root.SetArgs([]string{"remove", "ghcr.io/test:v1"})

	err := root.Execute()
	require.NoError(t, err)

	assert.Contains(t, out.String(), "Bundle removed successfully")
}
