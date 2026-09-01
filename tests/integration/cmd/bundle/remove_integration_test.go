// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

// Cluster-free integration tests for remove command wiring and prompting.
// These tests use the built UDS CLI binary for consistency with deploy tests.

package bundle_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

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

func (s *RemoveSuite) TestRemoveCommand_LocalArtifact() {
	artifact := createInspectArtifact(s.T())
	cmd := exec.Command(s.uds, "bundle", "remove", artifact, "--skip-signature-verification", "--prompt")
	cmd.Stdin = strings.NewReader("n\n")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err, "uds bundle remove output:\n%s", output)
	assert.Contains(s.T(), string(output), "inspect-integration")
	assert.Contains(s.T(), string(output), "Remove this bundle?")
	assert.Contains(s.T(), string(output), "removal cancelled")
}
func (s *RemoveSuite) TestRemoveCommand_OCIArtifact() {
	artifact := createInspectArtifact(s.T())
	registryHost := testutil.StartLocalRegistry(s.T())
	ref := fmt.Sprintf("%s/test/remove:v1.0.0", registryHost)
	config := &bundlepkg.UDSBundleConfig{
		Options: &bundlepkg.ConfigOptions{
			Architecture: runtime.GOARCH,
			Concurrency:  10,
			PlainHTTP:    true,
			TmpDir:       s.T().TempDir(),
		},
	}
	_, err := bundlepkg.Push(s.T().Context(), artifact, ref, bundlepkg.PushOptions{Config: config})
	require.NoError(s.T(), err)
	cmd := exec.Command(s.uds, "bundle", "remove", ref, "--plain-http", "--skip-signature-verification", "--prompt")
	cmd.Stdin = strings.NewReader("n\n")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err, "uds bundle remove output:\n%s", output)
	assert.Contains(s.T(), string(output), "inspect-integration")
	assert.Contains(s.T(), string(output), "Remove this bundle?")
	assert.Contains(s.T(), string(output), "removal cancelled")
}
func (s *RemoveSuite) TestRemoveCommand_InvalidArtifactReference() {
	path := filepath.Join(s.T().TempDir(), "not-a-bundle.txt")
	require.NoError(s.T(), os.WriteFile(path, []byte("not a bundle"), 0o600))
	cmd := exec.Command(s.uds, "bundle", "remove", path)
	output, err := cmd.CombinedOutput()
	require.Error(s.T(), err)
	assert.Contains(
		s.T(),
		string(output),
		"expected file named 'bundle.uds.hcl', got: not-a-bundle.txt",
	)
}
func (s *RemoveSuite) TestRemoveCommand_UnavailableArtifactReferences() {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "local tarball",
			args: []string{"bundle", "remove", filepath.Join(s.T().TempDir(), "missing.tar.zst", "--skip-signature-verification")},
		},
		{
			name: "OCI artifact",
			args: []string{"bundle", "remove", fmt.Sprintf("%s/test/missing:v1", testutil.StartLocalRegistry(s.T())), "--plain-http", "--skip-signature-verification"},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			cmd := exec.Command(s.uds, tt.args...)
			output, err := cmd.CombinedOutput()
			require.Error(s.T(), err)
			assert.Contains(s.T(), strings.ToLower(string(output)), "not found")
		})
	}
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
