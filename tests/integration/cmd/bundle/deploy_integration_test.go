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
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

// DeploySuite is a testify suite for cluster-free deploy command integration tests.
type DeploySuite struct {
	suite.Suite
	uds string
}

// SetupSuite resolves the UDS CLI binary path once for the whole suite.
func (s *DeploySuite) SetupSuite() {
	s.uds = testutil.UDSCLIPath(s.T(), "run via 'maru run test:integration'")
}

// TestDeploySuite is the entry point that runs the suite.
func TestDeploySuite(t *testing.T) {
	suite.Run(t, new(DeploySuite))
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

// TestDeployCommand_PackagesFlagInHelp verifies that --packages is documented
// in the deploy help output.
func (s *DeploySuite) TestDeployCommand_PackagesFlagInHelp() {
	cmd := exec.Command(s.uds, "bundle", "deploy", "--help")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err, "help should succeed")

	assert.Contains(s.T(), string(output), "--packages",
		"help output should document --packages flag")
	assert.Contains(s.T(), string(output), "--force",
		"help output should document --force flag")
}

func (s *DeploySuite) TestDevDeployCommand_HelpAndRouting() {
	cmd := exec.Command(s.uds, "bundle", "dev", "deploy", "--help")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err, "development deploy help should succeed")
	assert.Contains(s.T(), string(output), "bundle definition")
	assert.Contains(s.T(), string(output), "--packages")
	assert.Contains(s.T(), string(output), "--force")
	assert.Contains(s.T(), string(output), "--concurrency")
	assert.Contains(s.T(), string(output), "--prompt")

	bundlePath := testutil.TestDataPath("bundles/deploy/init")
	rootDeploy := exec.Command(s.uds, "bundle", "deploy", bundlePath)
	rootOutput, rootErr := rootDeploy.CombinedOutput()
	require.Error(s.T(), rootErr)
	assert.Contains(s.T(), string(rootOutput), "uds bundle dev deploy")

	artifact := filepath.Join(s.T().TempDir(), "bundle.tar.zst")
	require.NoError(s.T(), os.WriteFile(artifact, []byte("test"), 0o600))
	devDeploy := exec.Command(s.uds, "bundle", "dev", "deploy", artifact)
	devOutput, devErr := devDeploy.CombinedOutput()
	require.Error(s.T(), devErr)
	assert.Contains(s.T(), string(devOutput), "uds bundle deploy")
}

// TestDevDeployCommand_InvalidPackagesFlag verifies that specifying a non-existent
// package name via --packages fails with a clear error, without requiring a cluster.
func (s *DeploySuite) TestDevDeployCommand_InvalidPackagesFlag() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	cmd := exec.Command(s.uds, "bundle", "dev", "deploy", bundlePath, "--packages", "nonexistent")
	output, err := cmd.CombinedOutput()

	assert.Error(s.T(), err, "deploy with invalid packages should fail")
	assert.Contains(s.T(), string(output), "unknown packages",
		"error should mention the unknown package")
}

// TestDevDeployCommand_InvalidPackagesFlagWithPromptDeclined verifies that an
// invalid --packages selection fails even under --prompt: the package check
// runs before (and independently of) the confirmation prompt, so the prompt
// never participates in the rejection.
func (s *DeploySuite) TestDevDeployCommand_InvalidPackagesFlagWithPromptDeclined() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	cmd := exec.Command(s.uds, "bundle", "dev", "deploy", bundlePath, "--packages", "nonexistent", "--prompt")
	// "n" would decline the prompt, but validation rejects --packages first, so this
	// is never read — present to prove the failure is independent of prompt input.
	cmd.Stdin = strings.NewReader("n\n")
	output, err := cmd.CombinedOutput()

	assert.Error(s.T(), err, "invalid packages should fail before the prompt is reached")
	assert.Contains(s.T(), string(output), "unknown packages",
		"error should mention the unknown package")
}

// TestDevDeployCommand_DisplaysPreview verifies that development deploy shows the bundle preview
// before prompting for confirmation when --prompt is used.
func (s *DeploySuite) TestDevDeployCommand_DisplaysPreview() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	// Use --prompt to enable interactive mode, pipe "n\n" to decline the deployment
	cmd := exec.Command(s.uds, "bundle", "dev", "deploy", bundlePath, "--prompt")
	cmd.Stdin = strings.NewReader("n\n")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err)

	outputStr := string(output)

	// Bundle metadata is now logged to stderr via slog.Info (not formatted with BUNDLE METADATA header)
	// CombinedOutput captures both stdout and stderr, so we verify the slog message
	assert.Contains(s.T(), outputStr, "bundle to deploy")
	assert.Contains(s.T(), outputStr, "k3d-core-init")

	// Verify confirmation prompt was shown
	assert.Contains(s.T(), outputStr, "Deploy this bundle?")
	assert.Contains(s.T(), outputStr, "deployment cancelled")
}

// TestDevDeployCommand_CancellationDoesNotDeploy verifies that declining the confirmation
// prompt prevents the deployment from starting when --prompt is used.
func (s *DeploySuite) TestDevDeployCommand_CancellationDoesNotDeploy() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	// Use --prompt to enable interactive mode, pipe "n\n" to decline the deployment
	cmd := exec.Command(s.uds, "bundle", "dev", "deploy", bundlePath, "--prompt")
	cmd.Stdin = strings.NewReader("n\n")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err)

	outputStr := string(output)

	// Verify deployment was cancelled (slog outputs lowercase message)
	assert.Contains(s.T(), outputStr, "deployment cancelled")

	// Verify deployment did not start (no deployment level logs)
	assert.NotContains(s.T(), outputStr, "starting deployment level")
}

// TestDevDeployCommand_NonInteractiveDefault verifies that the default (non-interactive)
// mode proceeds without showing a confirmation prompt. The deploy will fail because
// there is no cluster, but the output should show deployment starting without a prompt.
func (s *DeploySuite) TestDevDeployCommand_NonInteractiveDefault() {
	bundlePath := prepareClusterFreeVariablesBundle(s.T())
	configPath := testutil.TestDataPath("bundles/deploy/variables/config.uds.hcl")

	// No --prompt flag, no stdin - non-interactive by default
	cmd := exec.Command(
		s.uds,
		"bundle", "dev", "deploy", bundlePath,
		"--config", configPath,
		"--packages", "podinfo",
		"--force",
	)
	output, _ := cmd.CombinedOutput()

	outputStr := string(output)
	assert.Contains(s.T(), outputStr, "variables-test")
	assert.NotContains(s.T(), outputStr, "Deploy this bundle?")
	assert.Contains(s.T(), outputStr, "starting deployment level")
	assert.Contains(s.T(), outputStr, "missing-podinfo-package",
		"deploy should stop at package loading before any cluster interaction")
}

// TestDeployCommand_ConfigFlagInHelp verifies that the --config flag is documented.
func (s *DeploySuite) TestDeployCommand_ConfigFlagInHelp() {
	cmd := exec.Command(s.uds, "bundle", "deploy", "--help")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err, "help should succeed")

	assert.Contains(s.T(), string(output), "--config",
		"help output should document --config flag")
}

// TestDevDeployCommand_InvalidConfigPath verifies that a non-existent --config path
// fails at the validation phase without requiring a cluster.
func (s *DeploySuite) TestDevDeployCommand_InvalidConfigPath() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	cmd := exec.Command(s.uds, "bundle", "dev", "deploy", bundlePath, "--config", "/nonexistent/config.uds.hcl")
	output, err := cmd.CombinedOutput()

	assert.Error(s.T(), err, "deploy with non-existent config should fail")
	assert.Contains(s.T(), string(output), "config",
		"error output should mention config")
}

// TestDevDeployCommand_InvalidConfigSyntax verifies that a syntactically invalid
// config.uds.hcl file is rejected with an HCL parse error before deployment begins.
func (s *DeploySuite) TestDevDeployCommand_InvalidConfigSyntax() {
	// Write an invalid HCL file
	dir := s.T().TempDir()
	invalidConfig := dir + "/config.uds.hcl"
	if err := os.WriteFile(invalidConfig, []byte("this is not valid HCL }{"), 0o600); err != nil {
		s.T().Fatalf("failed to write invalid config: %v", err)
	}

	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	cmd := exec.Command(s.uds, "bundle", "dev", "deploy", bundlePath, "--config", invalidConfig)
	output, err := cmd.CombinedOutput()

	assert.Error(s.T(), err, "deploy with invalid config HCL should fail")
	assert.Contains(s.T(), string(output), "failed to parse",
		"error output should indicate a parse error")
}

// TestDevDeployCommand_ListVariableTemplating verifies that list/object variables in
// config.uds.hcl flow through values_files templating without errors. Deploy proceeds
// past templating; later cluster/registry-access failure is tolerated.
func (s *DeploySuite) TestDevDeployCommand_ListVariableTemplating() {
	bundlePath := prepareClusterFreeVariablesBundle(s.T())
	configPath := testutil.TestDataPath("bundles/deploy/variables/config.uds.hcl")

	cmd := exec.Command(
		s.uds,
		"bundle", "dev", "deploy", bundlePath,
		"--config", configPath,
		"--packages", "podinfo",
		"--force",
	)
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	assert.NotContains(s.T(), outputStr, "unsupported variable type",
		"variable conversion must accept lists/objects/nested maps")
	assert.NotContains(s.T(), outputStr, "failed to convert variables",
		"variable conversion must succeed")
	assert.NotContains(s.T(), outputStr, "failed to template values files",
		"templating must succeed")
	assert.NotContains(s.T(), outputStr, "map has no entry for key",
		"all referenced variables must be present")
	assert.Contains(s.T(), outputStr, "starting deployment level",
		"deploy must reach the deployment phase after successful templating")
	assert.Contains(s.T(), outputStr, "missing-podinfo-package",
		"deploy should stop at package loading before any cluster interaction")
}

// TestDevDeployCommand_MissingTemplateVariable verifies that a values_files template
// referencing an undefined variable fails with missingkey=error before any registry
// or cluster access is attempted.
func (s *DeploySuite) TestDevDeployCommand_MissingTemplateVariable() {
	bundlePath := testutil.TestDataPath("bundles/deploy/variables")
	// config-missing-var.uds.hcl has other_var but not cluster_name,
	// so k3d.yaml (which uses {{ .vars.cluster_name }}) should fail at template time.
	configPath := testutil.TestDataPath("bundles/deploy/variables/config-missing-var.uds.hcl")

	cmd := exec.Command(s.uds, "bundle", "dev", "deploy", bundlePath, "--config", configPath)
	output, err := cmd.CombinedOutput()

	assert.Error(s.T(), err, "deploy with missing template variable should fail")
	assert.Contains(s.T(), string(output), "map has no entry for key",
		"error should indicate the missing variable")
}

func prepareClusterFreeVariablesBundle(t *testing.T) string {
	t.Helper()

	bundlePath := testutil.PrepareBundleDir(t, "bundles/deploy/variables")
	bundleFile := filepath.Join(bundlePath, "bundle.uds.hcl")
	content, err := os.ReadFile(bundleFile)
	require.NoError(t, err)

	const packageSource = "./zarf-package-podinfo-${sys.arch}-0.1.0.tar.zst"
	require.Contains(t, string(content), packageSource)
	content = []byte(strings.Replace(
		string(content),
		packageSource,
		"./missing-podinfo-package-${sys.arch}.tar.zst",
		1,
	))
	require.NoError(t, os.WriteFile(bundleFile, content, 0o600))
	return bundlePath
}
