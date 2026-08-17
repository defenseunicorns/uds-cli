// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package bundle_test

import (
	"encoding/json"
	"fmt"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/cli"
	"github.com/defenseunicorns/uds-cli/internal/cli/bundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/tests/testutil"
	"gopkg.in/yaml.v3"
)

func TestInspectCommand_Integration(t *testing.T) {
	bundlePath := createInspectArtifact(t)

	streams, _, out, _ := iostreams.NewTestIOStreams()

	root := cli.NewRootCommand(streams)
	root.SetArgs([]string{"bundle", "inspect", bundlePath, "--skip-signature-verification"})

	err := root.Execute()
	require.NoError(t, err)

	output := out.String()

	// Check fields through the built-artifact path.
	assert.Contains(t, output, "Name:")
	assert.Contains(t, output, "inspect-integration")
	assert.Contains(t, output, "Version:")
	assert.Contains(t, output, "1.0.0")

	// Verify Packages section
	assert.Contains(t, output, "Packages (1)")
	assert.Contains(t, output, "pkg")
	assert.Contains(t, output, "skipped")
}

func TestInspectCommand_StructuredOutput_Integration(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			artifact := createInspectArtifact(t)
			streams, _, out, _ := iostreams.NewTestIOStreams()
			root := cli.NewRootCommand(streams)
			root.SetArgs([]string{"bundle", "inspect", artifact, "--skip-signature-verification", "--output", format})
			require.NoError(t, root.Execute())

			var result bundlepkg.InspectResult
			if format == "json" {
				require.NoError(t, json.Unmarshal(out.Bytes(), &result))
			} else {
				require.NoError(t, yaml.Unmarshal(out.Bytes(), &result))
			}

			assert.Equal(t, "inspect-integration", result.Name)
			assert.Equal(t, "1.0.0", result.Version)
			assert.NotEmpty(t, result.ArtifactDigest)
			require.NotNil(t, result.BundleSignature)
			assert.Equal(t, bundlepkg.BundleSignatureStatusSkipped, result.BundleSignature.Status)
			require.Len(t, result.Packages, 1)
			assert.Equal(t, "pkg", result.Packages[0].Name)
			require.NotNil(t, result.Packages[0].Signature)
			assert.Equal(t, bundlepkg.PackageSigningStatusSigned, result.Packages[0].Signature.Signed)
			assert.Equal(t, bundlepkg.PackageVerificationStatusSkipped, result.Packages[0].Signature.Verification)
		})
	}
}

func TestInspectOCICommand_Integration(t *testing.T) {
	artifact := createInspectArtifact(t)
	registryHost := startLocalRegistry(t)
	ref := fmt.Sprintf("%s/test/inspect:v1.0.0", registryHost)
	config := &bundlepkg.UDSBundleConfig{
		Global: &bundlepkg.GlobalOptions{LogLevel: "info"},
		Options: &bundlepkg.ConfigOptions{
			Architecture: runtime.GOARCH,
			Concurrency:  10,
			PlainHTTP:    true,
			TmpDir:       t.TempDir(),
		},
	}
	_, err := bundlepkg.Push(t.Context(), artifact, ref, bundlepkg.PushOptions{Config: config})
	require.NoError(t, err)

	streams, _, out, _ := iostreams.NewTestIOStreams()
	root := cli.NewRootCommand(streams)
	root.SetArgs([]string{"bundle", "inspect", ref, "--plain-http", "--skip-signature-verification", "--output", "json"})
	require.NoError(t, root.Execute())

	var result bundlepkg.InspectResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &result))
	assert.Equal(t, "inspect-integration", result.Name)
	assert.Equal(t, "1.0.0", result.Version)
	assert.NotEmpty(t, result.ArtifactDigest)
	require.Len(t, result.Packages, 1)
	assert.Equal(t, "pkg", result.Packages[0].Name)
	require.NotNil(t, result.Packages[0].Signature)
	assert.Equal(t, bundlepkg.PackageSigningStatusSigned, result.Packages[0].Signature.Signed)
}

// Note: Error cases (OCI reference, file not found) are tested in unit tests
// because CheckErr calls os.Exit(1) which would terminate the test process.

func TestDeployCommand_Integration(t *testing.T) {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	streams, in, _, errOut := iostreams.NewTestIOStreams()
	// Simulate user declining the deployment via --prompt
	in.WriteString("n\n")

	// Use root command because --prompt is a root-level persistent flag
	root := cli.NewRootCommand(streams)
	root.SetArgs([]string{"bundle", "dev", "deploy", bundlePath, "--prompt"})

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
	root := cli.NewRootCommand(streams)
	root.SetArgs([]string{"bundle", "dev", "deploy", bundlePath, "--prompt"})

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
	root.SetArgs([]string{"pull", ref, "--output-dir", outDir, "--plain-http", "--skip-signature-verification"})

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
		SkipSignatureVerification: true,
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

	root := cli.NewRootCommand(streams)
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

	root := cli.NewRootCommand(streams)
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

	root := cli.NewRootCommand(streams)
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
	root := cli.NewRootCommand(streams)
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

	root := cli.NewRootCommand(streams)
	root.SetArgs([]string{"bundle", "remove", bundlePath, "--packages", "init,uds_k3d_dev", "--prompt"})

	err := root.Execute()
	require.NoError(t, err)

	errOutput := errOut.String()
	assert.Contains(t, errOutput, "Remove this bundle?")
}
