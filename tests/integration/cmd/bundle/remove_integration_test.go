// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

// Integration tests for the remove command that require a running k3d cluster.
// These tests use the built UDS CLI binary for consistency with deploy tests.

package bundle_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

// RemoveSuite is a testify suite for remove integration tests.
type RemoveSuite struct {
	suite.Suite
	uds string
}

// SetupSuite resolves the UDS CLI binary path once for the whole suite.
func (s *RemoveSuite) SetupSuite() {
	s.uds = testutil.UDSCLIPath(s.T(), "run via 'maru run test:integration'")
}

// TearDownTest runs automatically after every test in the suite.
func (s *RemoveSuite) TearDownTest() {
	testutil.DeleteK3dCluster(s.T(), "uds")
}

// TestRemoveSuite is the entry point that runs the suite.
func TestRemoveSuite(t *testing.T) {
	suite.Run(t, new(RemoveSuite))
}

// TestRemoveCommand_WithPromptFlag verifies the --prompt flag behavior
// by checking that the help output documents it.
func (s *RemoveSuite) TestRemoveCommand_WithPromptFlag() {
	cmd := exec.Command(s.uds, "bundle", "remove", "--help")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err, "help should succeed")

	assert.Contains(s.T(), string(output), "--prompt",
		"help output should document --prompt flag")
	assert.Contains(s.T(), string(output), "--packages",
		"help output should document --packages flag")
}

// TestRemoveCommand_DisplaysPreview verifies that remove command shows bundle preview
// before prompting for confirmation when --prompt is used.
func (s *RemoveSuite) TestRemoveCommand_DisplaysPreview() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	cmd := exec.Command(s.uds, "bundle", "remove", bundlePath, "--prompt")
	cmd.Stdin = strings.NewReader("n\n")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err)

	outputStr := string(output)

	assert.Contains(s.T(), outputStr, "bundle to remove")
	assert.Contains(s.T(), outputStr, "k3d-core-init")
	assert.Contains(s.T(), outputStr, "Remove this bundle?")
	assert.Contains(s.T(), outputStr, "removal cancelled")
}

// TestRemoveCommand_CancellationDoesNotRemove verifies that declining the confirmation
// prompt prevents the removal from starting when --prompt is used.
func (s *RemoveSuite) TestRemoveCommand_CancellationDoesNotRemove() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	cmd := exec.Command(s.uds, "bundle", "remove", bundlePath, "--prompt")
	cmd.Stdin = strings.NewReader("n\n")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err)

	outputStr := string(output)

	assert.Contains(s.T(), outputStr, "removal cancelled")
	assert.NotContains(s.T(), outputStr, "removing package")
}

// TestRemoveCommand_InvalidPackagesFlag verifies that specifying a non-existent
// package name via --packages fails with a clear error.
func (s *RemoveSuite) TestRemoveCommand_InvalidPackagesFlag() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	cmd := exec.Command(s.uds, "bundle", "remove", bundlePath, "--packages", "nonexistent")
	output, err := cmd.CombinedOutput()

	assert.Error(s.T(), err, "remove with invalid packages should fail")
	assert.Contains(s.T(), string(output), "unknown packages",
		"error should mention the unknown package")
}

// TestDeployAndRemoveBundle deploys the init bundle and then removes it.
// This test verifies the full deploy-remove lifecycle, including that every
// package is actually removed (not silently skipped). The fixture's HCL labels
// (`uds_k3d_dev`, `init`) intentionally do not all match the underlying Zarf
// metadata.name (`uds-k3d`, `init`), so this also exercises the
// label-vs-Zarf-name divergence in RemovePackage.
func (s *RemoveSuite) TestDeployAndRemoveBundle() {
	testutil.CheckDockerRunning(s.T(), "Docker is not running; deploy tests require Docker for k3d")

	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	// Delete any existing cluster
	testutil.DeleteK3dCluster(s.T(), "uds")

	// 1. Deploy the init bundle
	s.T().Log("Deploying init bundle...")
	deployCmd := exec.Command(s.uds, "bundle", "deploy", bundlePath)
	deployCmd.Stdout = os.Stdout
	deployCmd.Stderr = os.Stderr
	deployCmd.Env = os.Environ()

	err := deployCmd.Run()
	require.NoError(s.T(), err, "bundle deploy should succeed")

	// 2. Verify deployment
	k8s := testutil.NewK8sClientOrSkip(s.T())
	k8s.AssertNamespaceExists("zarf")
	// The init Zarf package writes its state secret to zarf/zarf-package-init
	// on a successful deploy. If this is missing, the deploy never landed and
	// the rest of the test would give us a false positive.
	k8s.AssertSecretExists("zarf", "zarf-package-init")

	// 3. Remove the bundle. Use -o json so we can parse the result and assert
	// on Removed/Skipped counts (text output is human-formatted).
	s.T().Log("Removing bundle...")
	var stdout, stderr bytes.Buffer
	removeCmd := exec.Command(s.uds, "bundle", "remove", bundlePath, "-o", "json")
	removeCmd.Stdout = io.MultiWriter(&stdout, os.Stdout)
	removeCmd.Stderr = io.MultiWriter(&stderr, os.Stderr)
	removeCmd.Env = os.Environ()

	err = removeCmd.Run()
	require.NoError(s.T(), err, "bundle remove should succeed")

	// 4. Parse the JSON result and assert both packages were actually removed.
	// Skipping is not an error, so without this assertion a regression where
	// pkg.Name != Zarf metadata.name would silently pass.
	var result bundle.RemoveResult
	s.Require().NoError(json.Unmarshal(stdout.Bytes(), &result),
		"remove output should be valid JSON: %s", stdout.String())

	s.Equal("k3d-core-init", result.BundleName)
	s.Equal(2, result.Removed,
		"both packages (uds_k3d_dev, init) should have been removed, not skipped")
	s.Equal(0, result.Skipped,
		"no packages should be skipped on a clean teardown")

	// 5. State secret for the init package should be gone.
	k8s.AssertSecretNotExists("zarf", "zarf-package-init")

	s.T().Log("Deploy and remove integration test completed successfully")
}
