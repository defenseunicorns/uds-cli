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
	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

func TestInspectCommand_Integration(t *testing.T) {
	bundlePath := testutil.TestDataPath("bundles/spec-compliant/bundle.uds.hcl")

	streams, _, out, _ := iostreams.NewTestIOStreams()

	root := bundle.NewBundleCommand(streams)
	root.SetArgs([]string{"inspect", bundlePath})

	err := root.Execute()
	require.NoError(t, err)

	output := out.String()

	// Verify result fields (now via ResourcePrinter text output)
	assert.Contains(t, output, "Name:")
	assert.Contains(t, output, "uds-core-example")
	assert.Contains(t, output, "Version:")
	assert.Contains(t, output, "0.1.0")

	// Verify PACKAGES section
	assert.Contains(t, output, "PACKAGES (3)")

	// Verify locals were fully resolved
	assert.NotContains(t, output, "${local.")

	// Verify resolved source URLs contain the expected OCI prefix (version is managed by Renovate)
	assert.Contains(t, output, "oci://ghcr.io/defenseunicorns/packages/uds/core-base:")
	assert.NotContains(t, output, "${local.version}")

	// Verify depends_on and valuesFiles
	assert.Contains(t, output, "DependsOn:")
	assert.Contains(t, output, "core_base")
	assert.Contains(t, output, "Value Files:")
	assert.Contains(t, output, "values/loki.yaml, values/vector.yaml")
	assert.Contains(t, output, "values/monitoring.yaml")
	assert.Contains(t, output, "Namespace:")
	assert.Contains(t, output, "monitoring")
}

// Note: Error cases (OCI reference, file not found) are tested in unit tests
// because CheckErr calls os.Exit(1) which would terminate the test process.

func TestDeployCommand_Integration(t *testing.T) {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	streams, in, _, errOut := iostreams.NewTestIOStreams()
	// Simulate user declining the deployment via --prompt
	in.WriteString("n\n")

	// Use root command because --prompt is a root-level persistent flag
	root := cmd.NewRootCommand(streams)
	root.SetArgs([]string{"bundle", "deploy", bundlePath, "--prompt"})

	err := root.Execute()
	require.NoError(t, err)

	// The confirmation prompt is written to ErrOut (IOStreams stderr)
	errOutput := errOut.String()
	assert.Contains(t, errOutput, "Deploy this bundle?")
}

func TestDeployCommand_WithBundleFile_Integration(t *testing.T) {
	bundlePath := testutil.TestDataPath("bundles/deploy/init/bundle.uds.hcl")

	streams, in, _, errOut := iostreams.NewTestIOStreams()
	// Simulate user declining the deployment via --prompt
	in.WriteString("n\n")

	// Use root command because --prompt is a root-level persistent flag
	root := cmd.NewRootCommand(streams)
	root.SetArgs([]string{"bundle", "deploy", bundlePath, "--prompt"})

	err := root.Execute()
	require.NoError(t, err)

	// The confirmation prompt is written to ErrOut (IOStreams stderr)
	errOutput := errOut.String()
	assert.Contains(t, errOutput, "Deploy this bundle?")
}

func TestPullCommand_Integration(t *testing.T) {
	// Push a bundle programmatically so we can test the pull cobra wiring.
	bundlePath := testutil.CreateBundleFromTestData(t, "bundles/create/init", runtime.GOARCH)
	registryHost := startLocalRegistry(t)
	ref := fmt.Sprintf("%s/test/k3d-core-init:v0.1.0", registryHost)

	_, err := bundlepkg.Push(t.Context(), bundlePath, ref, bundlepkg.PushOptions{
		Config: &bundlepkg.UDSBundleConfig{
			Global:  &bundlepkg.GlobalOptions{},
			Options: &bundlepkg.ConfigOptions{TmpDir: t.TempDir(), PlainHTTP: true, Concurrency: 10},
		},
	})
	require.NoError(t, err)

	outDir := t.TempDir()
	streams, _, out, _ := iostreams.NewTestIOStreams()
	root := bundle.NewBundleCommand(streams)
	root.SetArgs([]string{"pull", ref, "--output-dir", outDir, "--plain-http"})

	err = root.Execute()
	require.NoError(t, err)

	assert.Contains(t, out.String(), "OCI Reference:")
	assert.Contains(t, out.String(), "Output Path:")
}

func TestPushCommand_Integration(t *testing.T) {
	bundlePath := testutil.CreateBundleFromTestData(t, "bundles/create/init", runtime.GOARCH)
	registryHost := startLocalRegistry(t)
	ref := fmt.Sprintf("%s/test/k3d-core-init:v0.1.0", registryHost)

	streams, _, out, _ := iostreams.NewTestIOStreams()
	root := bundle.NewBundleCommand(streams)
	root.SetArgs([]string{"push", bundlePath, ref, "--plain-http"})

	err := root.Execute()
	require.NoError(t, err)

	assert.Contains(t, out.String(), "OCI Reference:")
}

func TestPushCommand_Integration_PlainHTTPAllowsTLS(t *testing.T) {
	bundlePath := testutil.CreateBundleFromTestData(t, "bundles/create/init", runtime.GOARCH)
	registryHost := startLocalTLSRegistry(t)
	ref := fmt.Sprintf("%s/test/k3d-core-init:v0.1.0", registryHost)

	streams, _, out, _ := iostreams.NewTestIOStreams()
	root := bundle.NewBundleCommand(streams)
	root.SetArgs([]string{"push", bundlePath, ref, "--plain-http", "--skip-tls-verify"})

	require.NoError(t, root.Execute())
	assert.Contains(t, out.String(), "OCI Reference:")
}

// TestPushPull_RoundTrip verifies that a bundle produced by Create can be pushed
// to a local OCI registry and pulled back, and that the pulled tarball contains
// exactly the same set of blob digests as the original.
func TestPushPull_RoundTrip(t *testing.T) {
	arch := runtime.GOARCH
	registryHost := startLocalRegistry(t)
	ref := fmt.Sprintf("%s/test/k3d-core-init:v0.1.0", registryHost)

	// create the bundle from test data.
	originalPath := testutil.CreateBundleFromTestData(t, "bundles/create/init", arch)
	assertValidBundleStructure(t, originalPath)

	// push the bundle to the local registry.
	_, err := bundlepkg.Push(t.Context(), originalPath, ref, bundlepkg.PushOptions{
		Config: &bundlepkg.UDSBundleConfig{
			Global:  &bundlepkg.GlobalOptions{},
			Options: &bundlepkg.ConfigOptions{TmpDir: t.TempDir(), PlainHTTP: true, Concurrency: 10},
		},
	})
	require.NoError(t, err, "Push should succeed against local registry")

	// pull the bundle from the local registry.
	outDir := t.TempDir()
	pullResult, err := bundlepkg.Pull(t.Context(), ref, outDir, bundlepkg.PullOptions{
		Config: &bundlepkg.UDSBundleConfig{
			Global:  &bundlepkg.GlobalOptions{},
			Options: &bundlepkg.ConfigOptions{TmpDir: t.TempDir(), PlainHTTP: true, Architecture: arch, Concurrency: 10},
		},
	})
	require.NoError(t, err, "Pull should succeed against local registry")

	assertValidBundleStructure(t, pullResult.OutputPath)
	assertBundleTarballsEqual(t, originalPath, pullResult.OutputPath)
}

func TestRemoveCommand_Integration(t *testing.T) {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	streams, in, _, errOut := iostreams.NewTestIOStreams()
	// Simulate user declining the removal via --prompt
	in.WriteString("n\n")

	root := cmd.NewRootCommand(streams)
	root.SetArgs([]string{"bundle", "remove", bundlePath, "--prompt"})

	err := root.Execute()
	require.NoError(t, err)

	errOutput := errOut.String()
	assert.Contains(t, errOutput, "Remove this bundle?")
}

func TestRemoveCommand_WithBundleFile_Integration(t *testing.T) {
	bundlePath := testutil.TestDataPath("bundles/deploy/init/bundle.uds.hcl")

	streams, in, _, errOut := iostreams.NewTestIOStreams()
	in.WriteString("n\n")

	root := cmd.NewRootCommand(streams)
	root.SetArgs([]string{"bundle", "remove", bundlePath, "--prompt"})

	err := root.Execute()
	require.NoError(t, err)

	errOutput := errOut.String()
	assert.Contains(t, errOutput, "Remove this bundle?")
}

func TestRemoveCommand_PackagesFlag_Integration(t *testing.T) {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	streams, in, _, errOut := iostreams.NewTestIOStreams()
	in.WriteString("n\n")

	root := cmd.NewRootCommand(streams)
	root.SetArgs([]string{"bundle", "remove", bundlePath, "--packages", "init", "--prompt"})

	err := root.Execute()
	require.NoError(t, err)

	errOutput := errOut.String()
	assert.Contains(t, errOutput, "Remove this bundle?")
}

// Note: Invalid --packages error cases are tested in unit tests because
// CheckErr calls os.Exit(1) which would terminate the test process.

func TestRemoveCommand_HelpOutput_Integration(t *testing.T) {
	streams, _, out, errOut := iostreams.NewTestIOStreams()

	// Use root command so --prompt (a root-level persistent flag) is visible
	root := cmd.NewRootCommand(streams)
	root.SetArgs([]string{"bundle", "remove", "--help"})

	err := root.Execute()
	require.NoError(t, err)

	output := out.String() + errOut.String()
	assert.Contains(t, output, "--packages")
	assert.Contains(t, output, "--prompt")
	assert.Contains(t, output, "Remove a UDS bundle from a Kubernetes cluster")
}

func TestRemoveCommand_CustomDirWithPackages_Integration(t *testing.T) {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	streams, in, _, errOut := iostreams.NewTestIOStreams()
	in.WriteString("n\n")

	root := cmd.NewRootCommand(streams)
	root.SetArgs([]string{"bundle", "remove", bundlePath, "--packages", "init,uds_k3d_dev", "--prompt"})

	err := root.Execute()
	require.NoError(t, err)

	errOutput := errOut.String()
	assert.Contains(t, errOutput, "Remove this bundle?")
}
