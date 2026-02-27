// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

// Integration tests in this file use the built UDS CLI binary (via exec.Command) rather than
// calling Go commands directly (e.g. bundle.NewDeployCommand().Execute()).
//
// This is required because Zarf's packager.Deploy() internally shells out for actions and
// wait-for operations (e.g. "<zarfCommand> tools kubectl wait ..."). The callback command is
// resolved by GetFinalExecutableCommand() which relies on config.ActionsCommandZarfPrefix
// being set via ldflags at build time ("-X '...ActionsCommandZarfPrefix=zarf'"). Without
// these ldflags (as in a "go test" binary), Zarf falls back to calling a system "zarf" binary
// which doesn't exist. Even non-deploying tests use the binary approach for consistency.

package bundle_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// checkDockerRunning verifies Docker is running.
func checkDockerRunning(t *testing.T) {
	t.Helper()
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		t.Skip("Docker is not running — deploy tests require Docker for k3d")
	}
}

// deleteK3dCluster deletes the k3d cluster created by the test.
func deleteK3dCluster(t *testing.T, clusterName string) {
	t.Helper()
	t.Logf("Cleaning up k3d cluster: %s", clusterName)

	cmd := exec.Command("k3d", "cluster", "delete", clusterName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		t.Logf("Warning: failed to delete k3d cluster %s: %v", clusterName, err)
	}
}

// TestDeployInitBundle deploys the init bundle and verifies the cluster is functional.
// This test:
// 1. Deploys the k3d-core-init bundle using the built UDS CLI binary
// 2. Verifies the zarf namespace exists using client-go
// 3. Cleans up the k3d cluster after the test
//
// Note: This test uses the Zarf Go library (vendored) for package deployment.
// No Zarf CLI installation is required.
func TestDeployInitBundle(t *testing.T) {
	// Skip if prerequisites are not met
	uds := udsCLIPath(t)
	checkDockerRunning(t)

	bundlePath := testDataPath(t, "bundles/deploy/init")
	clusterName := "uds" // The uds-k3d package creates a cluster named "uds"

	// Ensure we clean up the cluster after the test
	t.Cleanup(func() {
		deleteK3dCluster(t, clusterName)
	})

	// Delete any existing cluster with the same name
	deleteK3dCluster(t, clusterName)

	// 1. Deploy the init bundle using the built binary
	t.Log("Deploying init bundle...")
	cmd := exec.Command(uds, "bundle", "deploy", bundlePath, "--confirm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	err := cmd.Run()
	require.NoError(t, err, "bundle deploy should succeed")

	// 2. Verify the cluster is reachable and Zarf components are deployed
	// Using client-go instead of kubectl
	t.Log("Verifying cluster state using client-go...")
	k8s := NewK8sClient(t)

	// 2a. Verify zarf namespace exists
	k8s.AssertNamespaceExists("zarf")

	// 2b. Verify zarf-state secret exists
	k8s.AssertSecretExists("zarf", "zarf-state")

	// 2c. Verify zarf-agent deployment exists
	k8s.AssertDeploymentExists("zarf", "agent-hook")

	t.Log("✓ Deploy integration test completed successfully")
}

// TestDeployCommand_WithConfirmFlag verifies the --confirm flag behavior
// by checking that the help output documents it.
func TestDeployCommand_WithConfirmFlag(t *testing.T) {
	uds := udsCLIPath(t)

	// Run with --help to verify the flag is recognized
	cmd := exec.Command(uds, "bundle", "deploy", "--help")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "help should succeed")

	// Verify --confirm flag is documented
	assert.True(t, strings.Contains(string(output), "--confirm") || strings.Contains(string(output), "-y"),
		"help output should document --confirm flag")
}

// TestDeployCommand_DisplaysPreview verifies that deploy command shows bundle preview
// before prompting for confirmation.
func TestDeployCommand_DisplaysPreview(t *testing.T) {
	uds := udsCLIPath(t)
	bundlePath := testDataPath(t, "bundles/deploy/init")

	// Pipe "n\n" to stdin to decline the deployment
	cmd := exec.Command(uds, "bundle", "deploy", bundlePath)
	cmd.Stdin = strings.NewReader("n\n")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err)

	outputStr := string(output)

	// Verify bundle metadata is displayed
	assert.Contains(t, outputStr, "BUNDLE METADATA")
	assert.Contains(t, outputStr, "Name:        k3d-core-init")

	// Verify packages are displayed
	assert.Contains(t, outputStr, "PACKAGES (2)")
	assert.Contains(t, outputStr, "uds_k3d_dev")
	assert.Contains(t, outputStr, "init")

	// Verify confirmation prompt was shown
	assert.Contains(t, outputStr, "Deploy this bundle?")
	assert.Contains(t, outputStr, "Deployment cancelled")
}

// TestDeployCommand_CancellationDoesNotDeploy verifies that declining the confirmation
// prompt prevents the deployment from starting.
func TestDeployCommand_CancellationDoesNotDeploy(t *testing.T) {
	uds := udsCLIPath(t)
	bundlePath := testDataPath(t, "bundles/deploy/init")

	// Pipe "n\n" to stdin to decline the deployment
	cmd := exec.Command(uds, "bundle", "deploy", bundlePath)
	cmd.Stdin = strings.NewReader("n\n")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err)

	outputStr := string(output)

	// Verify deployment was cancelled
	assert.Contains(t, outputStr, "Deployment cancelled")

	// Verify deployment did not start
	assert.NotContains(t, outputStr, "Starting deployment")
	assert.NotContains(t, outputStr, "Deployment Level")
}
