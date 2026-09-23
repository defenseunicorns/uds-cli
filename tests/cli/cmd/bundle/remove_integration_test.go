// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cli

// Cluster-free integration tests for remove command wiring and prompting.

package bundle_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/stretchr/testify/suite"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

// RemoveSuite is a testify suite for cluster-free remove command integration tests.
type RemoveSuite struct {
	suite.Suite
}

func TestRemoveSuite(t *testing.T) {
	suite.Run(t, new(RemoveSuite))
}

func (s *RemoveSuite) TestRemoveCommand_WithPromptFlag() {
	output, err := executeCLI(s.T(), "", "bundle", "remove", "--help")
	s.Require().NoError(err, "help should succeed")
	s.Contains(output, "--prompt", "help output should document --prompt flag")
	s.Contains(output, "--packages", "help output should document --packages flag")
}

func (s *RemoveSuite) TestDevRemoveCommand_WithPromptFlag() {
	output, err := executeCLI(s.T(), "", "bundle", "dev", "remove", "--help")
	s.Require().NoError(err, "help should succeed")
	s.Contains(output, "--prompt", "help output should document --prompt flag")
	s.Contains(output, "--packages", "help output should document --packages flag")
}

func (s *RemoveSuite) TestRemoveCommand_DisplaysPreview() {
	artifact := createInspectArtifact(s.T())
	output, err := executeCLI(s.T(), "n\n", "bundle", "remove", artifact, "--skip-signature-verification", "--prompt")
	s.Require().NoError(err)

	s.Contains(output, "bundle to remove")
	s.Contains(output, "inspect-integration")
	s.Contains(output, "Remove this bundle?")
	s.Contains(output, "removal cancelled")
}

func (s *RemoveSuite) TestDevRemoveCommand_DisplaysPreview() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")
	output, err := executeCLI(s.T(), "n\n", "bundle", "dev", "remove", bundlePath, "--prompt")
	s.Require().NoError(err)

	s.Contains(output, "bundle to remove")
	s.Contains(output, "k3d-core-init")
	s.Contains(output, "Remove this bundle?")
	s.Contains(output, "removal cancelled")
}

func (s *RemoveSuite) TestRemoveCommand_CancellationDoesNotRemove() {
	artifact := createInspectArtifact(s.T())
	output, err := executeCLI(s.T(), "n\n", "bundle", "remove", artifact, "--skip-signature-verification", "--prompt")
	s.Require().NoError(err)

	s.Contains(output, "removal cancelled")
	s.NotContains(output, "removing package")
}

func (s *RemoveSuite) TestDevRemoveCommand_CancellationDoesNotRemove() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")
	output, err := executeCLI(s.T(), "n\n", "bundle", "dev", "remove", bundlePath, "--prompt")
	s.Require().NoError(err)

	s.Contains(output, "removal cancelled")
	s.NotContains(output, "removing package")
}

func (s *RemoveSuite) TestRemoveCommand_LocalArtifact() {
	artifact := createInspectArtifact(s.T())
	output, err := executeCLI(s.T(), "n\n", "bundle", "remove", artifact, "--skip-signature-verification", "--prompt")
	s.Require().NoError(err, "uds bundle remove output: %s", output)
	s.Contains(output, "inspect-integration")
	s.Contains(output, "Remove this bundle?")
	s.Contains(output, "removal cancelled")
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
	s.Require().NoError(err)

	output, err := executeCLI(s.T(), "n\n", "bundle", "remove", ref, "--plain-http", "--skip-signature-verification", "--prompt")
	s.Require().NoError(err, "uds bundle remove output: %s", output)
	s.Contains(output, "inspect-integration")
	s.Contains(output, "Remove this bundle?")
	s.Contains(output, "removal cancelled")
}

func (s *RemoveSuite) TestRemoveCommand_InvalidArtifactReference() {
	path := filepath.Join(s.T().TempDir(), "not-a-bundle.txt")
	s.Require().NoError(os.WriteFile(path, []byte("not a bundle"), 0o600))
	_, err := executeCLI(s.T(), "", "bundle", "remove", path)
	s.Require().Error(err)
	s.Contains(err.Error(), "expected file named 'bundle.uds.hcl', got: not-a-bundle.txt")
}

func (s *RemoveSuite) TestRemoveCommand_UnavailableArtifactReferences() {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "local tarball",
			args: []string{"bundle", "remove", filepath.Join(s.T().TempDir(), "missing.tar.zst"), "--skip-signature-verification"},
		},
		{
			name: "OCI artifact",
			args: []string{"bundle", "remove", fmt.Sprintf("%s/test/missing:v1", testutil.StartLocalRegistry(s.T())), "--plain-http", "--skip-signature-verification"},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			_, err := executeCLI(s.T(), "", tt.args...)
			s.Require().Error(err)
			s.Contains(strings.ToLower(err.Error()), "not found")
		})
	}
}

func (s *RemoveSuite) TestRemoveCommand_InvalidPackagesFlag() {
	artifact := createInspectArtifact(s.T())
	_, err := executeCLI(s.T(), "", "bundle", "remove", artifact, "--packages", "nonexistent", "--skip-signature-verification")
	s.Require().Error(err, "remove with invalid packages should fail")
	s.Contains(err.Error(), "unknown packages", "error should mention the unknown package")
}

func (s *RemoveSuite) TestDevRemoveCommand_InvalidPackagesFlag() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")
	_, err := executeCLI(s.T(), "", "bundle", "dev", "remove", bundlePath, "--packages", "nonexistent")
	s.Require().Error(err, "dev remove with invalid packages should fail")
	s.Contains(err.Error(), "unknown packages", "error should mention the unknown package")
}
