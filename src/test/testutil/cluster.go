// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	developmentClusterName = "uds"
	commandTimeout         = 2 * time.Minute
	clusterSetupTimeout    = 10 * time.Minute
	apiReadyTimeout        = 3 * time.Minute
	initTimeout            = 20 * time.Minute
)

var invalidNamespaceCharacters = regexp.MustCompile(`[^a-z0-9]+`)

// ClusterSuite contains paths shared by tests using the local UDS dev-stack cluster.
// Callers must treat these paths as read-only and copy fixtures before editing.
type ClusterSuite struct {
	CLIPath        string
	KubeconfigPath string
	WorkspacePath  string
	FixtureRoot    string
	BootstrapPath  string
}

// SetupDevelopmentCluster prepares the retained local UDS dev-stack cluster.
// When absent or not Zarf initialized, it bootstraps the cluster with uds-k3d.
func SetupDevelopmentCluster(ctx context.Context) (_ *ClusterSuite, setupErr error) {
	if ctx == nil {
		return nil, errors.New("setup development cluster: context is required")
	}

	cliPath, err := configuredCLIPath()
	if err != nil {
		return nil, err
	}
	for _, tool := range []string{"docker", "k3d", "kubectl"} {
		if _, err := exec.LookPath(tool); err != nil {
			return nil, fmt.Errorf("required cluster test tool %q: %w", tool, err)
		}
	}

	workspace, err := os.MkdirTemp("", "uds-cli-cluster-suite-")
	if err != nil {
		return nil, fmt.Errorf("create cluster suite workspace: %w", err)
	}
	defer func() {
		if setupErr != nil {
			_ = os.RemoveAll(workspace)
		}
	}()

	fixtureRoot := filepath.Join(workspace, "fixtures")
	suite := &ClusterSuite{
		CLIPath:        cliPath,
		KubeconfigPath: filepath.Join(workspace, "kubeconfig.yaml"),
		WorkspacePath:  workspace,
		FixtureRoot:    fixtureRoot,
		BootstrapPath:  TestDataPath("bundles/00-cluster-bootstrap"),
	}

	if err := runWithTimeout(ctx, commandTimeout, "docker", "info"); err != nil {
		return nil, fmt.Errorf("validate Docker: %w", err)
	}
	exists, err := developmentClusterExists(ctx)
	if err != nil {
		return nil, err
	}
	if !exists {
		fmt.Printf("Cluster setup: bootstrapping %q with uds-k3d and Zarf init\n", developmentClusterName)
		if err := bootstrapDevelopmentCluster(ctx, suite); err != nil {
			return nil, err
		}
	} else {
		fmt.Printf("Cluster setup: reusing %q\n", developmentClusterName)
		if err := runWithTimeout(ctx, clusterSetupTimeout, "k3d", "cluster", "start", developmentClusterName, "--wait", "--timeout", clusterSetupTimeout.String()); err != nil {
			return nil, fmt.Errorf("start development cluster: %w", err)
		}
	}

	fmt.Printf("Cluster setup: writing kubeconfig for %q\n", developmentClusterName)
	if err := runWithTimeout(ctx, commandTimeout, "k3d", "kubeconfig", "write", developmentClusterName,
		"--output", suite.KubeconfigPath, "--overwrite"); err != nil {
		return nil, fmt.Errorf("write retained cluster kubeconfig: %w", err)
	}
	client, err := kubernetesClient(suite.KubeconfigPath)
	if err != nil {
		return nil, err
	}
	if err := waitForKubernetesAPI(ctx, client); err != nil {
		return nil, err
	}
	if err := validateZarfInitialized(ctx, client); err != nil {
		fmt.Printf("Cluster setup: Zarf is not ready; rebuilding %q with uds-k3d\n", developmentClusterName)
		if err := bootstrapDevelopmentCluster(ctx, suite); err != nil {
			return nil, err
		}
		if err := runWithTimeout(ctx, commandTimeout, "k3d", "kubeconfig", "write", developmentClusterName,
			"--output", suite.KubeconfigPath, "--overwrite"); err != nil {
			return nil, fmt.Errorf("write bootstrapped cluster kubeconfig: %w", err)
		}
		client, err = kubernetesClient(suite.KubeconfigPath)
		if err != nil {
			return nil, err
		}
		if err := waitForKubernetesAPI(ctx, client); err != nil {
			return nil, err
		}
		if err := validateZarfInitialized(ctx, client); err != nil {
			return nil, fmt.Errorf("validate Zarf after uds-k3d bootstrap: %w", err)
		}
	}

	return suite, nil
}

func bootstrapDevelopmentCluster(ctx context.Context, suite *ClusterSuite) error {
	fmt.Printf("Cluster setup: deploying uds-k3d bootstrap bundle for %q\n", developmentClusterName)
	commands := bootstrapCommands(suite.WorkspacePath, suite.BootstrapPath)
	result, err := runCommand(ctx, suite.CLIPath, CommandOptions{Dir: suite.WorkspacePath}, commands[0]...)
	if err != nil {
		return fmt.Errorf("create development cluster bootstrap bundle: %w\n%s", err, result)
	}

	bundles, err := filepath.Glob(commands[1][1])
	if err != nil {
		return fmt.Errorf("find development cluster bootstrap bundle: %w", err)
	}
	if len(bundles) != 1 {
		return fmt.Errorf("expected one development cluster bootstrap bundle, found %d", len(bundles))
	}

	args := commands[1]
	args[1] = bundles[0]
	result, err = runCommand(ctx, suite.CLIPath, CommandOptions{Dir: suite.WorkspacePath}, args...)
	if err != nil {
		return fmt.Errorf("bootstrap development cluster %q with uds-k3d: %w\n%s", developmentClusterName, err, result)
	}
	return nil
}

func bootstrapCommands(workspace, bootstrapPath string) [][]string {
	return [][]string{
		{"create", bootstrapPath, "--confirm", "--output", workspace},
		{"deploy", filepath.Join(workspace, "uds-bundle-test-cluster-bootstrap-*.tar.zst"), "--confirm", "--no-progress"},
	}
}

// CommandOptions returns child-only environment and workspace settings for the suite.
func (s *ClusterSuite) CommandOptions() CommandOptions {
	return CommandOptions{
		Dir: s.WorkspacePath,
		Env: map[string]string{"KUBECONFIG": s.KubeconfigPath},
	}
}

// Close releases suite-local files. It never deletes the developer's UDS cluster.
func (s *ClusterSuite) Close(_ context.Context) error {
	defer os.RemoveAll(s.WorkspacePath)
	return nil
}

// AllocateNamespace reserves a test-unique namespace and registers bounded cleanup.
//
// It deliberately does not create the namespace. A normal Zarf deploy must create
// and manage it so Zarf also writes the private-registry pull secret. Pre-creating
// the namespace without taking ownership causes Zarf v0.82 to skip that secret.
func AllocateNamespace(t *testing.T, suite *ClusterSuite) (string, *Kubernetes) {
	t.Helper()

	client := NewKubernetes(t, suite.KubeconfigPath)
	name, err := uniqueNamespaceName(t.Name())
	if err != nil {
		t.Fatalf("generate namespace name: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), namespaceCleanupTimeout)
		defer cancel()
		if err := deleteNamespaceAndWait(ctx, client.client, name); err != nil {
			t.Errorf("clean up namespace %q: %v", name, err)
		}
	})
	return name, client
}

func configuredCLIPath() (string, error) {
	path := os.Getenv("UDS_CLI_PATH")
	if path == "" {
		return "", errors.New("UDS_CLI_PATH must be configured for cluster integration tests")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve UDS_CLI_PATH %q: %w", path, err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("validate UDS_CLI_PATH %q: %w", absPath, err)
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("UDS_CLI_PATH %q is not an executable file", absPath)
	}
	return absPath, nil
}

func developmentClusterExists(ctx context.Context) (bool, error) {
	output, err := runWithTimeoutOutput(ctx, commandTimeout, "k3d", "cluster", "list", "--output", "json")
	if err != nil {
		return false, fmt.Errorf("list k3d clusters: %w", err)
	}
	var clusters []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(output), &clusters); err != nil {
		return false, fmt.Errorf("parse k3d cluster list: %w", err)
	}
	for _, cluster := range clusters {
		if cluster.Name == developmentClusterName {
			return true, nil
		}
	}
	return false, nil
}

func waitForKubernetesAPI(ctx context.Context, client kubernetes.Interface) error {
	waitCtx, cancel := context.WithTimeout(ctx, apiReadyTimeout)
	defer cancel()
	err := wait.PollUntilContextCancel(waitCtx, time.Second, true, func(ctx context.Context) (bool, error) {
		result := client.Discovery().RESTClient().Get().AbsPath("/readyz").Do(ctx)
		if result.Error() != nil {
			return false, nil //nolint:nilerr // API connection errors are expected while k3d starts.
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("wait for Kubernetes API: %w", err)
	}
	return nil
}

func validateZarfInitialized(ctx context.Context, client kubernetes.Interface) error {
	registryExists, err := deploymentExists(ctx, client, "zarf", "zarf-docker-registry")
	if err != nil {
		return err
	}
	agentExists, err := deploymentExists(ctx, client, "zarf", "agent-hook")
	if err != nil {
		return err
	}
	if registryExists && agentExists {
		return nil
	}
	if registryExists != agentExists {
		return errors.New("UDS development cluster has partial Zarf initialization")
	}
	return errors.New("UDS development cluster is not Zarf initialized; initialize the dev stack before running cluster integration tests")
}

func deploymentExists(ctx context.Context, client kubernetes.Interface, namespace, name string) (bool, error) {
	_, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get deployment %s/%s: %w", namespace, name, err)
	}
	return true, nil
}

func uniqueNamespaceName(testName string) (string, error) {
	sanitized := invalidNamespaceCharacters.ReplaceAllString(strings.ToLower(testName), "-")
	sanitized = strings.Trim(sanitized, "-")
	if sanitized == "" {
		sanitized = "test"
	}
	const maxTestNameLength = 46
	if len(sanitized) > maxTestNameLength {
		sanitized = strings.Trim(sanitized[:maxTestNameLength], "-")
	}
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return "uds-it-" + sanitized + "-" + hex.EncodeToString(random), nil
}

func runWithTimeout(ctx context.Context, timeout time.Duration, command string, args ...string) error {
	_, err := runWithTimeoutOutput(ctx, timeout, command, args...)
	return err
}

func runWithTimeoutOutput(ctx context.Context, timeout time.Duration, command string, args ...string) (string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s %s: %w\n%s", command, strings.Join(args, " "), err, output)
	}
	return string(output), nil
}

func runCommand(ctx context.Context, command string, opts CommandOptions, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = opts.Dir
	cmd.Env = childEnv(opts.Env)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s %s: %w\n%s", command, strings.Join(args, " "), err, output)
	}
	return string(output), nil
}
