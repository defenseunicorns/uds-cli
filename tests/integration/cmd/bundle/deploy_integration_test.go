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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

// DeploySuite is a testify suite for deploy integration tests.
// TearDownTest runs automatically after every test method, ensuring the
// k3d cluster is always cleaned up regardless of test outcome.
type DeploySuite struct {
	suite.Suite
	uds            string
	podinfoZarfPkg string
}

// SetupSuite resolves the UDS CLI binary path and builds shared test prerequisites once for the whole suite.
func (s *DeploySuite) SetupSuite() {
	s.uds = testutil.UDSCLIPath(s.T(), "run via 'maru run test:integration'")

	// Pre-build the podinfo Zarf package for the variables bundle test. This will be cleaned up in the TearDownSuite.
	arch := runtime.GOARCH
	varsBundleDir := testutil.TestDataPath("bundles/deploy/variables")
	buildZarfPackage(s.T(), s.uds, testutil.TestDataPath("packages/podinfo"), varsBundleDir, arch)
	s.podinfoZarfPkg = filepath.Join(varsBundleDir, "zarf-package-podinfo-"+arch+"-0.1.0.tar.zst")
}

// TearDownSuite cleans up suite-level prerequisites.
func (s *DeploySuite) TearDownSuite() {
	if s.podinfoZarfPkg != "" {
		_ = os.Remove(s.podinfoZarfPkg)
	}
}

// TearDownTest runs automatically after every test in the suite.
func (s *DeploySuite) TearDownTest() {
	testutil.DeleteK3dCluster(s.T(), "uds")
	testutil.DeleteK3dCluster(s.T(), "uds-vars-test")
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
	testutil.CheckDockerRunning(s.T(), "Docker is not running; deploy tests require Docker for k3d")

	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	// Delete any existing cluster with the same name before deploying
	testutil.DeleteK3dCluster(s.T(), "uds")

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
	k8s := testutil.NewK8sClientOrSkip(s.T())

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

// TestDeployCommand_InvalidPackagesFlag verifies that specifying a non-existent
// package name via --packages fails with a clear error, without requiring a cluster.
func (s *DeploySuite) TestDeployCommand_InvalidPackagesFlag() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	cmd := exec.Command(s.uds, "bundle", "deploy", bundlePath, "--packages", "nonexistent")
	output, err := cmd.CombinedOutput()

	assert.Error(s.T(), err, "deploy with invalid packages should fail")
	assert.Contains(s.T(), string(output), "unknown packages",
		"error should mention the unknown package")
}

// TestDeployCommand_InvalidPackagesFlagWithPromptDeclined verifies that an
// invalid --packages selection fails even under --prompt: the package check
// runs before (and independently of) the confirmation prompt, so the prompt
// never participates in the rejection.
func (s *DeploySuite) TestDeployCommand_InvalidPackagesFlagWithPromptDeclined() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	cmd := exec.Command(s.uds, "bundle", "deploy", bundlePath, "--packages", "nonexistent", "--prompt")
	// "n" would decline the prompt, but validation rejects --packages first, so this
	// is never read — present to prove the failure is independent of prompt input.
	cmd.Stdin = strings.NewReader("n\n")
	output, err := cmd.CombinedOutput()

	assert.Error(s.T(), err, "invalid packages should fail before the prompt is reached")
	assert.Contains(s.T(), string(output), "unknown packages",
		"error should mention the unknown package")
}

// TestDeployCommand_DisplaysPreview verifies that deploy command shows bundle preview
// before prompting for confirmation when --prompt is used.
func (s *DeploySuite) TestDeployCommand_DisplaysPreview() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	// Use --prompt to enable interactive mode, pipe "n\n" to decline the deployment
	cmd := exec.Command(s.uds, "bundle", "deploy", bundlePath, "--prompt")
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

// TestDeployCommand_CancellationDoesNotDeploy verifies that declining the confirmation
// prompt prevents the deployment from starting when --prompt is used.
func (s *DeploySuite) TestDeployCommand_CancellationDoesNotDeploy() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	// Use --prompt to enable interactive mode, pipe "n\n" to decline the deployment
	cmd := exec.Command(s.uds, "bundle", "deploy", bundlePath, "--prompt")
	cmd.Stdin = strings.NewReader("n\n")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err)

	outputStr := string(output)

	// Verify deployment was cancelled (slog outputs lowercase message)
	assert.Contains(s.T(), outputStr, "deployment cancelled")

	// Verify deployment did not start (no deployment level logs)
	assert.NotContains(s.T(), outputStr, "starting deployment level")
}

// TestDeployCommand_NonInteractiveDefault verifies that the default (non-interactive)
// mode proceeds without showing a confirmation prompt. The deploy will fail because
// there is no cluster, but the output should show deployment starting without a prompt.
func (s *DeploySuite) TestDeployCommand_NonInteractiveDefault() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	// No --prompt flag, no stdin - non-interactive by default
	cmd := exec.Command(s.uds, "bundle", "deploy", bundlePath)
	output, _ := cmd.CombinedOutput()

	outputStr := string(output)

	// Should show bundle info (via slog) and proceed to deployment without asking
	assert.Contains(s.T(), outputStr, "k3d-core-init")
	assert.NotContains(s.T(), outputStr, "Deploy this bundle?")
	assert.Contains(s.T(), outputStr, "starting deployment level")
}

// TestDeployCommand_ConfigFlagInHelp verifies that the --config flag is documented.
func (s *DeploySuite) TestDeployCommand_ConfigFlagInHelp() {
	cmd := exec.Command(s.uds, "bundle", "deploy", "--help")
	output, err := cmd.CombinedOutput()
	require.NoError(s.T(), err, "help should succeed")

	assert.Contains(s.T(), string(output), "--config",
		"help output should document --config flag")
}

// TestDeployCommand_InvalidConfigPath verifies that a non-existent --config path
// fails at the validation phase without requiring a cluster.
func (s *DeploySuite) TestDeployCommand_InvalidConfigPath() {
	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	cmd := exec.Command(s.uds, "bundle", "deploy", bundlePath, "--config", "/nonexistent/config.uds.hcl")
	output, err := cmd.CombinedOutput()

	assert.Error(s.T(), err, "deploy with non-existent config should fail")
	assert.Contains(s.T(), string(output), "config",
		"error output should mention config")
}

// TestDeployCommand_InvalidConfigSyntax verifies that a syntactically invalid
// config.uds.hcl file is rejected with an HCL parse error before deployment begins.
func (s *DeploySuite) TestDeployCommand_InvalidConfigSyntax() {
	// Write an invalid HCL file
	dir := s.T().TempDir()
	invalidConfig := dir + "/config.uds.hcl"
	if err := os.WriteFile(invalidConfig, []byte("this is not valid HCL }{"), 0o600); err != nil {
		s.T().Fatalf("failed to write invalid config: %v", err)
	}

	bundlePath := testutil.TestDataPath("bundles/deploy/init")

	cmd := exec.Command(s.uds, "bundle", "deploy", bundlePath, "--config", invalidConfig)
	output, err := cmd.CombinedOutput()

	assert.Error(s.T(), err, "deploy with invalid config HCL should fail")
	assert.Contains(s.T(), string(output), "failed to parse",
		"error output should indicate a parse error")
}

// TestDeployCommand_ListVariableTemplating verifies that list/object variables in
// config.uds.hcl flow through values_files templating without errors. Deploy proceeds
// past templating; later cluster/registry-access failure is tolerated.
func (s *DeploySuite) TestDeployCommand_ListVariableTemplating() {
	bundlePath := testutil.TestDataPath("bundles/deploy/variables")
	configPath := testutil.TestDataPath("bundles/deploy/variables/config.uds.hcl")

	cmd := exec.Command(s.uds, "bundle", "deploy", bundlePath, "--config", configPath)
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Templating must not fail — these are the negative assertions that prove
	// the parser accepts list/object/nested-map and the templater renders them.
	assert.NotContains(s.T(), outputStr, "unsupported variable type",
		"variable conversion must accept lists/objects/nested maps")
	assert.NotContains(s.T(), outputStr, "failed to convert variables",
		"variable conversion must succeed")
	assert.NotContains(s.T(), outputStr, "failed to template values files",
		"templating must succeed")
	assert.NotContains(s.T(), outputStr, "map has no entry for key",
		"all referenced variables must be present")

	// Positive marker: deploy reaches the deployment phase, proving parsing
	// and templating both succeeded.
	assert.Contains(s.T(), outputStr, "starting deployment level",
		"deploy must reach the deployment phase after successful templating")
}

// TestDeployCommand_MissingTemplateVariable verifies that a values_files template
// referencing an undefined variable fails with missingkey=error before any registry
// or cluster access is attempted.
func (s *DeploySuite) TestDeployCommand_MissingTemplateVariable() {
	bundlePath := testutil.TestDataPath("bundles/deploy/variables")
	// config-missing-var.uds.hcl has other_var but not cluster_name,
	// so k3d.yaml (which uses {{ .vars.cluster_name }}) should fail at template time.
	configPath := testutil.TestDataPath("bundles/deploy/variables/config-missing-var.uds.hcl")

	cmd := exec.Command(s.uds, "bundle", "deploy", bundlePath, "--config", configPath)
	output, err := cmd.CombinedOutput()

	assert.Error(s.T(), err, "deploy with missing template variable should fail")
	assert.Contains(s.T(), string(output), "map has no entry for key",
		"error should indicate the missing variable")
}

// TestDeployVariablesBundleWithPodinfo builds the podinfo Zarf package for the
// current architecture, deploys it via the variables bundle with a non-default
// config (1 replica, service disabled, custom annotations/tolerations), and
// validates the resulting cluster state.
func (s *DeploySuite) TestDeployVariablesBundleWithPodinfo() {
	testutil.CheckDockerRunning(s.T(), "Docker is not running; deploy tests require Docker for k3d")

	arch := runtime.GOARCH

	// Create a temp bundle dir with the values files and the built package.
	bundleTmpDir := s.T().TempDir()
	valuesDir := filepath.Join(bundleTmpDir, "values")
	require.NoError(s.T(), os.MkdirAll(valuesDir, 0o755))

	srcValuesDir := testutil.TestDataPath("bundles/deploy/variables/values")
	require.NoError(s.T(), os.CopyFS(valuesDir, os.DirFS(srcValuesDir)))

	// Build the podinfo Zarf package for the current arch directly into the bundle dir.
	buildZarfPackage(s.T(), s.uds, testutil.TestDataPath("packages/podinfo"), bundleTmpDir, arch)

	// Bundle: uds-k3d creates the cluster, init provides zarf-state, podinfo deploys the app.
	// sys.arch resolves to runtime.GOARCH automatically — no substitution required.
	bundleHCL := `# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name    = "variables-test"
  version = "0.1.0"
}

package "uds_k3d_dev" {
  source       = "oci://ghcr.io/defenseunicorns/packages/uds-k3d:0.20.0"
  values_files = ["values/k3d.yaml"]
}

package "init" {
  source     = "oci://ghcr.io/zarf-dev/packages/init:v0.75.1"
  depends_on = [package.uds_k3d_dev]
}

package "podinfo" {
  source       = "./zarf-package-podinfo-${sys.arch}-0.1.0.tar.zst"
  values_files = ["values/podinfo.yaml"]
  depends_on   = [package.init]
}
`
	require.NoError(s.T(), os.WriteFile(filepath.Join(bundleTmpDir, "bundle.uds.hcl"), []byte(bundleHCL), 0o644))

	configPath := testutil.TestDataPath("bundles/deploy/variables/full-config.uds.hcl")

	s.T().Log("Deploying variables bundle with podinfo...")
	cmd := exec.Command(s.uds, "bundle", "deploy", bundleTmpDir, "--config", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	require.NoError(s.T(), cmd.Run(), "bundle deploy should succeed")

	k8s := testutil.NewK8sClientOrSkip(s.T())

	k8s.WaitForDeploymentReady("podinfo", "podinfo", 5*time.Minute)

	// replica_count = 1 from config (default is 2)
	k8s.AssertDeploymentReplicas("podinfo", "podinfo", 1)

	// service_enabled = false — no Service resource created
	k8s.AssertServiceNotExists("podinfo", "podinfo")

	// annotations from config propagated to pod template
	k8s.AssertDeploymentPodAnnotation("podinfo", "podinfo", "app.kubernetes.io/managed-by", "uds")
	k8s.AssertDeploymentPodAnnotation("podinfo", "podinfo", "team", "platform")

	// tolerations from config applied to pod template
	k8s.AssertDeploymentPodToleration("podinfo", "podinfo",
		"node.kubernetes.io/not-ready", corev1.TolerationOpExists, corev1.TaintEffectNoExecute)

	s.T().Log("✓ Podinfo deploy validation complete")
}

// TestDeployFromArtifact deploys from a .tar.zst bundle artifact and verifies
// that configuration is applied using embedded defaults and the specified config
// file according to precedence order.
func (s *DeploySuite) TestDeployFromArtifact() {
	testutil.CheckDockerRunning(s.T(), "Docker is not running; deploy tests require Docker for k3d")

	// Build bundle artifact from variables test data (Zarf package already built in SetupSuite).
	artifactPath := testutil.CreateBundleFromTestData(s.T(), "bundles/deploy/variables", runtime.GOARCH)

	// Move the artifact into a clean temp dir to verify deploy works without bundle source files.
	deployDir := s.T().TempDir()
	deployArtifact := filepath.Join(deployDir, filepath.Base(artifactPath))
	require.NoError(s.T(), os.Rename(artifactPath, deployArtifact))

	configPath := testutil.TestDataPath("bundles/deploy/variables/config.uds.hcl")

	cmd := exec.Command(s.uds, "bundle", "deploy", deployArtifact, "--config", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	require.NoError(s.T(), cmd.Run(), "bundle deploy from artifact should succeed")

	k8s := testutil.NewK8sClientOrSkip(s.T())

	k8s.WaitForDeploymentReady("podinfo", "podinfo", 5*time.Minute)

	// replica_count = 1 from config (overrides embedded default of 2)
	k8s.AssertDeploymentReplicas("podinfo", "podinfo", 1)

	// Assert (non-overridden)config from defaults was applied
	// service_enabled = false from embedded defaults — no Service resource created
	// annotations from embedded defaults propagated to pod template
	// tolerations from embedded defaults applied to pod template
	k8s.AssertServiceNotExists("podinfo", "podinfo")
	k8s.AssertDeploymentPodAnnotation("podinfo", "podinfo", "app.kubernetes.io/managed-by", "uds")
	k8s.AssertDeploymentPodAnnotation("podinfo", "podinfo", "team", "platform")
	k8s.AssertDeploymentPodToleration("podinfo", "podinfo",
		"node.kubernetes.io/not-ready", corev1.TolerationOpExists, corev1.TaintEffectNoExecute)
}

// buildZarfPackage runs `uds zarf package create` on the given directory and places
// the resulting archive in outputDir. Fails the test if the build fails.
func buildZarfPackage(t *testing.T, uds, zarfYamlDir, outputDir, arch string) {
	t.Helper()

	cmd := exec.Command(uds, "zarf", "package", "create", zarfYamlDir,
		"--output", outputDir,
		"--architecture", arch,
		"--features", "values=true",
		"--confirm")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "zarf package build failed:\n%s", out)
}
