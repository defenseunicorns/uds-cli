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
	"github.com/stretchr/testify/suite"
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

// DeploySuite is a testify suite for deploy integration tests.
// TearDownTest runs automatically after every test method, ensuring the
// k3d cluster is always cleaned up regardless of test outcome.
type DeploySuite struct {
	suite.Suite
	uds string
}

// SetupSuite resolves the UDS CLI binary path once for the whole suite.
func (s *DeploySuite) SetupSuite() {
	s.uds = udsCLIPath(s.T())
}

// TearDownTest runs automatically after every test in the suite.
func (s *DeploySuite) TearDownTest() {
	deleteK3dCluster(s.T(), "uds")
}

// TestDeploySuite is the entry point that runs the suite.
func TestDeploySuite(t *testing.T) {
	suite.Run(t, new(DeploySuite))
}

// TestDeployInitBundle deploys the init bundle and verifies the cluster is functional.
// This test:
// 1. Deploys the k3d-core-init bundle using the built UDS CLI binary
// 2. Verifies the zarf namespace exists using client-go
// 3. Cleans up the k3d cluster after the test (via TearDownTest)
//
// Note: This test uses the Zarf Go library (vendored) for package deployment.
// No Zarf CLI installation is required.
func (s *DeploySuite) TestDeployInitBundle() {
	checkDockerRunning(s.T())

	bundlePath := testDataPath(s.T(), "bundles/deploy/init")

	// Delete any existing cluster with the same name before deploying
	deleteK3dCluster(s.T(), "uds")

	// 1. Deploy the init bundle using the built binary (non-interactive by default)
	s.T().Log("Deploying init bundle...")
	cmd := exec.Command(s.uds, "bundle", "deploy", bundlePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	err := cmd.Run()
	require.NoError(s.T(), err, "bundle deploy should succeed")

	// 2. Verify the cluster is reachable and Zarf components are deployed
	// Using client-go instead of kubectl
	s.T().Log("Verifying cluster state using client-go...")
	k8s := NewK8sClient(s.T())

	// 2a. Verify zarf namespace exists
	k8s.AssertNamespaceExists("zarf")

	// 2b. Verify zarf-state secret exists
	k8s.AssertSecretExists("zarf", "zarf-state")

	// 2c. Verify zarf-agent deployment exists
	k8s.AssertDeploymentExists("zarf", "agent-hook")

	s.T().Log("✓ Deploy integration test completed successfully")
}

// TestDeployCommand_WithPromptFlag verifies the --prompt flag behavior
// by checking that the help output documents it.
func (s *DeploySuite) TestDeployCommand_WithPromptFlag() {
	cmd := exec.Command(s.uds, "bundle", "deploy", "--help")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err, "help should succeed")

	assert.Contains(s.T(), string(output), "--prompt",
		"help output should document --prompt flag")
}

// TestDeployCommand_DisplaysPreview verifies that deploy command shows bundle preview
// before prompting for confirmation when --prompt is used.
func (s *DeploySuite) TestDeployCommand_DisplaysPreview() {
	bundlePath := testDataPath(s.T(), "bundles/deploy/init")

	// Use --prompt to enable interactive mode, pipe "n\n" to decline the deployment
	cmd := exec.Command(s.uds, "bundle", "deploy", bundlePath, "--prompt")
	cmd.Stdin = strings.NewReader("n\n")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err)

	outputStr := string(output)

	// Verify bundle metadata is displayed
	assert.Contains(s.T(), outputStr, "BUNDLE METADATA")
	assert.Contains(s.T(), outputStr, "Name:        k3d-core-init")

	// Verify packages are displayed
	assert.Contains(s.T(), outputStr, "PACKAGES (2)")
	assert.Contains(s.T(), outputStr, "uds_k3d_dev")
	assert.Contains(s.T(), outputStr, "init")

	// Verify confirmation prompt was shown
	assert.Contains(s.T(), outputStr, "Deploy this bundle?")
	assert.Contains(s.T(), outputStr, "Deployment cancelled")
}

// TestDeployCommand_CancellationDoesNotDeploy verifies that declining the confirmation
// prompt prevents the deployment from starting when --prompt is used.
func (s *DeploySuite) TestDeployCommand_CancellationDoesNotDeploy() {
	bundlePath := testDataPath(s.T(), "bundles/deploy/init")

	// Use --prompt to enable interactive mode, pipe "n\n" to decline the deployment
	cmd := exec.Command(s.uds, "bundle", "deploy", bundlePath, "--prompt")
	cmd.Stdin = strings.NewReader("n\n")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err)

	outputStr := string(output)

	// Verify deployment was cancelled
	assert.Contains(s.T(), outputStr, "Deployment cancelled")

	// Verify deployment did not start
	assert.NotContains(s.T(), outputStr, "starting deployment")
	assert.NotContains(s.T(), outputStr, "starting deployment level")
}

// TestDeployCommand_NonInteractiveDefault verifies that the default (non-interactive)
// mode proceeds without showing a confirmation prompt. The deploy will fail because
// there is no cluster, but the output should show deployment starting without a prompt.
func (s *DeploySuite) TestDeployCommand_NonInteractiveDefault() {
	bundlePath := testDataPath(s.T(), "bundles/deploy/init")

	// No --prompt flag, no stdin - non-interactive by default
	cmd := exec.Command(s.uds, "bundle", "deploy", bundlePath)
	output, _ := cmd.CombinedOutput()

	outputStr := string(output)

	// Should show bundle info and proceed to deployment without asking
	assert.Contains(s.T(), outputStr, "k3d-core-init")
	assert.NotContains(s.T(), outputStr, "Deploy this bundle?")
	assert.Contains(s.T(), outputStr, "starting deployment")
}
