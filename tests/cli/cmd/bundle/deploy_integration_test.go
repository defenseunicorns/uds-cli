// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cli

package bundle_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

// DeploySuite is a testify suite for cluster-free deploy command integration tests.
type DeploySuite struct {
	suite.Suite
}

func TestDeploySuite(t *testing.T) {
	suite.Run(t, new(DeploySuite))
}

func (s *DeploySuite) TestDeployCommand_WithPromptFlag() {
	output, err := executeCLI(s.T(), "", "bundle", "deploy", "--help")
	s.Require().NoError(err, "help should succeed")
	s.Contains(output, "--prompt", "help output should document --prompt flag")
}

func (s *DeploySuite) TestDeployCommand_PackagesFlagInHelp() {
	output, err := executeCLI(s.T(), "", "bundle", "deploy", "--help")
	s.Require().NoError(err, "help should succeed")
	s.Contains(output, "--packages", "help output should document --packages flag")
	s.Contains(output, "--force", "help output should document --force flag")
}

func (s *DeploySuite) TestDevDeployCommand_HelpAndRouting() {
	output, err := executeCLI(s.T(), "", "bundle", "dev", "deploy", "--help")
	s.Require().NoError(err, "development deploy help should succeed")
	s.Contains(output, "bundle definition")
	s.Contains(output, "--packages")
	s.Contains(output, "--force")
	s.Contains(output, "--concurrency")
	s.Contains(output, "--prompt")

	bundlePath := testutil.TestDataPath("bundles/deploy/init")
	_, rootErr := executeCLI(s.T(), "", "bundle", "deploy", bundlePath)
	s.Require().Error(rootErr)
	s.Contains(rootErr.Error(), "uds bundle dev deploy")

	artifact := filepath.Join(s.T().TempDir(), "bundle.tar.zst")
	s.Require().NoError(os.WriteFile(artifact, []byte("test"), 0o600))
	_, devErr := executeCLI(s.T(), "", "bundle", "dev", "deploy", artifact)
	s.Require().Error(devErr)
	s.Contains(devErr.Error(), "uds bundle deploy")
}

func (s *DeploySuite) TestDevDeployCommand_InvalidPackagesFlag() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")
	_, err := executeCLI(s.T(), "", "bundle", "dev", "deploy", bundlePath, "--packages", "nonexistent")
	s.Require().Error(err, "deploy with invalid packages should fail")
	s.Contains(err.Error(), "unknown packages", "error should mention the unknown package")
}

func (s *DeploySuite) TestDevDeployCommand_InvalidPackagesFlagWithPromptDeclined() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")
	// Validation rejects --packages before reading this input.
	_, err := executeCLI(s.T(), "n\n", "bundle", "dev", "deploy", bundlePath, "--packages", "nonexistent", "--prompt")
	s.Require().Error(err, "invalid packages should fail before the prompt is reached")
	s.Contains(err.Error(), "unknown packages", "error should mention the unknown package")
}

func (s *DeploySuite) TestDevDeployCommand_DisplaysPreview() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")
	output, err := executeCLI(s.T(), "n\n", "bundle", "dev", "deploy", bundlePath, "--prompt")
	s.Require().NoError(err)

	s.Contains(output, "bundle to deploy")
	s.Contains(output, "k3d-core-init")
	s.Contains(output, "Deploy this bundle?")
	s.Contains(output, "deployment cancelled")
}

func (s *DeploySuite) TestDevDeployCommand_CancellationDoesNotDeploy() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")
	output, err := executeCLI(s.T(), "n\n", "bundle", "dev", "deploy", bundlePath, "--prompt")
	s.Require().NoError(err)

	s.Contains(output, "deployment cancelled")
	s.NotContains(output, "starting deployment level")
}

func (s *DeploySuite) TestDevDeployCommand_NonInteractiveDefault() {
	bundlePath := prepareClusterFreeVariablesBundle(s.T())
	configPath := testutil.TestDataPath("bundles/deploy/variables/config.uds.hcl")

	output, _ := executeCLI(s.T(), "", "bundle", "dev", "deploy", bundlePath,
		"--config", configPath, "--packages", "podinfo", "--force")

	s.Contains(output, "variables-test")
	s.NotContains(output, "Deploy this bundle?")
	s.Contains(output, "starting deployment level")
	s.Contains(output, "missing-podinfo-package",
		"deploy should stop at package loading before any cluster interaction")
}

func (s *DeploySuite) TestDeployCommand_ConfigFlagInHelp() {
	output, err := executeCLI(s.T(), "", "bundle", "deploy", "--help")
	s.Require().NoError(err, "help should succeed")
	s.Contains(output, "--config", "help output should document --config flag")
}

func (s *DeploySuite) TestDevDeployCommand_InvalidConfigPath() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")
	_, err := executeCLI(s.T(), "", "bundle", "dev", "deploy", bundlePath, "--config", "/nonexistent/config.uds.hcl")
	s.Require().Error(err, "deploy with non-existent config should fail")
	s.Contains(err.Error(), "config", "error output should mention config")
}

func (s *DeploySuite) TestDevDeployCommand_InvalidConfigSyntax() {
	dir := s.T().TempDir()
	invalidConfig := filepath.Join(dir, "config.uds.hcl")
	s.Require().NoError(os.WriteFile(invalidConfig, []byte("this is not valid HCL }{"), 0o600))

	bundlePath := testutil.TestDataPath("bundles/deploy/init")
	_, err := executeCLI(s.T(), "", "bundle", "dev", "deploy", bundlePath, "--config", invalidConfig)
	s.Require().Error(err, "deploy with invalid config HCL should fail")
	s.Contains(err.Error(), "failed to parse", "error output should indicate a parse error")
}

func (s *DeploySuite) TestDevDeployCommand_ListVariableTemplating() {
	bundlePath := prepareClusterFreeVariablesBundle(s.T())
	configPath := testutil.TestDataPath("bundles/deploy/variables/config.uds.hcl")

	output, _ := executeCLI(s.T(), "", "bundle", "dev", "deploy", bundlePath,
		"--config", configPath, "--packages", "podinfo", "--force")

	s.NotContains(output, "unsupported variable type",
		"variable conversion must accept lists/objects/nested maps")
	s.NotContains(output, "failed to convert variables",
		"variable conversion must succeed")
	s.NotContains(output, "failed to template values files",
		"templating must succeed")
	s.NotContains(output, "map has no entry for key",
		"all referenced variables must be present")
	s.Contains(output, "starting deployment level",
		"deploy must reach the deployment phase after successful templating")
	s.Contains(output, "missing-podinfo-package",
		"deploy should stop at package loading before any cluster interaction")
}

func (s *DeploySuite) TestDevDeployCommand_SetVariables() {
	bundlePath := prepareClusterFreeVariablesBundle(s.T())
	configPath := testutil.TestDataPath("bundles/deploy/variables/config.uds.hcl")
	valuesPath := filepath.Join(bundlePath, "values", "podinfo.yaml")
	values, err := os.ReadFile(valuesPath)
	s.Require().NoError(err)
	values = append(values, []byte(`
cliString: {{ printf "%q" .vars.cli_string }}
cliNumber: {{ .vars.cli_number }}
cliBoolean: {{ .vars.cli_boolean }}
cliObject: {{ printf "%q" .vars.cli_object.name }}
`)...)
	s.Require().NoError(os.WriteFile(valuesPath, values, 0o600))

	output, err := executeCLI(s.T(), "", "bundle", "dev", "deploy", bundlePath,
		"--config", configPath,
		"--set", "cli_string=cli-test",
		"--set", "cli_number=2",
		"--set", "cli_boolean=false",
		"--set", `cli_object={ name = "cli" }`,
		"--set", `log_level="debug"`,
		"--packages", "podinfo",
		"--force",
	)

	s.Require().Error(err, "deploy should stop when loading the intentionally missing package")
	s.NotContains(output, "parsing --set variable")
	s.NotContains(output, "failed to template values files")
	s.NotContains(output, "map has no entry for key")
	s.Contains(output, "starting deployment level")
	s.Contains(output, "missing-podinfo-package")
}

func (s *DeploySuite) TestDevDeployCommand_MissingTemplateVariable() {
	bundlePath := testutil.TestDataPath("bundles/deploy/variables")
	configPath := testutil.TestDataPath("bundles/deploy/variables/config-missing-var.uds.hcl")

	_, err := executeCLI(s.T(), "", "bundle", "dev", "deploy", bundlePath, "--config", configPath)
	s.Require().Error(err, "deploy with missing template variable should fail")
	s.Contains(err.Error(), "map has no entry for key",
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
	//nolint:gosec // bundleFile is created below t.TempDir().
	require.NoError(t, os.WriteFile(bundleFile, content, 0o600))
	return bundlePath
}
