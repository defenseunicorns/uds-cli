// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const udsCoreSmokeBundle = "k3d-core-slim-dev:latest"

// DisposableCluster contains paths and clients scoped to one smoke scenario.
type DisposableCluster struct {
	Name           string
	CLIPath        string
	KubeconfigPath string
	WorkspacePath  string
	BundlePath     string

	t *testing.T
}

// CreateDisposableCluster creates an isolated k3d cluster and registers its deletion.
func CreateDisposableCluster(t *testing.T, name string) DisposableCluster {
	t.Helper()

	cliPath, err := configuredCLIPath()
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"docker", "k3d", "kubectl"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("required smoke test tool %q: %v", tool, err)
		}
	}

	clusterName, err := uniqueDisposableClusterName(name)
	if err != nil {
		t.Fatalf("generate disposable cluster name: %v", err)
	}
	workspace := t.TempDir()
	cluster := DisposableCluster{
		Name:           clusterName,
		CLIPath:        cliPath,
		KubeconfigPath: filepath.Join(workspace, "kubeconfig.yaml"),
		WorkspacePath:  workspace,
		BundlePath:     udsCoreSmokeBundle,
		t:              t,
	}

	if err := runWithTimeout(t.Context(), commandTimeout, "docker", "info"); err != nil {
		t.Fatalf("validate Docker: %v", err)
	}
	if err := runWithTimeout(t.Context(), clusterSetupTimeout, "k3d", "cluster", "create", cluster.Name,
		"--wait", "--timeout", clusterSetupTimeout.String(),
		"--kubeconfig-update-default=false", "--kubeconfig-switch-context=false"); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), clusterSetupTimeout)
		_ = runWithTimeout(cleanupCtx, clusterSetupTimeout, "k3d", "cluster", "delete", cluster.Name)
		cancel()
		t.Fatalf("create disposable cluster %q: %v", cluster.Name, err)
	}
	t.Cleanup(func() { cluster.cleanup() })

	if err := runWithTimeout(t.Context(), commandTimeout, "k3d", "kubeconfig", "write", cluster.Name,
		"--output", cluster.KubeconfigPath, "--overwrite"); err != nil {
		t.Fatalf("write disposable cluster kubeconfig: %v", err)
	}
	client, err := kubernetesClient(cluster.KubeconfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := waitForKubernetesAPI(t.Context(), client); err != nil {
		t.Fatal(err)
	}

	return cluster
}

// CommandOptions returns child-only environment and workspace settings for the cluster.
func (c DisposableCluster) CommandOptions() CommandOptions {
	return CommandOptions{
		Dir: c.WorkspacePath,
		Env: map[string]string{"KUBECONFIG": c.KubeconfigPath},
	}
}

// Kubernetes creates assertions bound to the cluster's test-local kubeconfig.
func (c DisposableCluster) Kubernetes() *Kubernetes {
	c.t.Helper()
	return NewKubernetes(c.t, c.KubeconfigPath)
}

func (c DisposableCluster) cleanup() {
	c.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), clusterSetupTimeout)
	defer cancel()
	if c.t.Failed() {
		c.collectDiagnostics(ctx)
	}
	if err := runWithTimeout(ctx, clusterSetupTimeout, "k3d", "cluster", "delete", c.Name); err != nil {
		c.t.Errorf("delete disposable cluster %q: %v", c.Name, err)
	}
}

func (c DisposableCluster) collectDiagnostics(ctx context.Context) {
	c.t.Helper()

	commands := []struct {
		name string
		args []string
	}{
		{name: "kubectl", args: []string{"--kubeconfig", c.KubeconfigPath, "get", "deployments,statefulsets,pods", "--all-namespaces", "-o", "wide"}},
		{name: "kubectl", args: []string{"--kubeconfig", c.KubeconfigPath, "get", "events", "--all-namespaces", "--sort-by=.lastTimestamp"}},
		{name: "docker", args: []string{"logs", "--tail", "200", "k3d-" + c.Name + "-server-0"}},
		{name: "k3d", args: []string{"cluster", "list"}},
	}
	for _, command := range commands {
		output, err := runWithTimeoutOutput(ctx, commandTimeout, command.name, command.args...)
		if err != nil {
			c.t.Logf("smoke diagnostics %s %s failed: %v", command.name, strings.Join(command.args, " "), err)
			continue
		}
		c.t.Logf("smoke diagnostics %s %s:\n%s", command.name, strings.Join(command.args, " "), output)
	}
}

func uniqueDisposableClusterName(prefix string) (string, error) {
	sanitized := invalidNamespaceCharacters.ReplaceAllString(strings.ToLower(prefix), "-")
	sanitized = strings.Trim(sanitized, "-")
	if sanitized == "" {
		sanitized = "uds-smoke"
	}
	if sanitized == developmentClusterName {
		return "", fmt.Errorf("reserved development cluster name %q", developmentClusterName)
	}
	const maxPrefixLength = 40
	if len(sanitized) > maxPrefixLength {
		sanitized = strings.Trim(sanitized[:maxPrefixLength], "-")
	}
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return sanitized + "-" + hex.EncodeToString(random), nil
}
