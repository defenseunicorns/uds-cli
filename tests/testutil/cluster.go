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
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
)

var invalidDNSLabelChars = regexp.MustCompile(`[^a-z0-9-]+`)

// ResolveUDSCLIPath resolves the CLI binary configured for integration tests.
func ResolveUDSCLIPath() (string, error) {
	path := os.Getenv("UDS_CLI_PATH")
	if path == "" {
		return "", errors.New("UDS_CLI_PATH is not set; run 'maru run build-local' first")
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("locate UDS CLI at %q: %w", path, err)
	}
	return path, nil
}

// CheckClusterPrerequisites verifies that Docker and k3d are available.
func CheckClusterPrerequisites(ctx context.Context) error {
	for _, name := range []string{"docker", "k3d"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("locate %s: %w", name, err)
		}
	}
	if _, err := commandOutput(ctx, os.Environ(), "docker", "info"); err != nil {
		return fmt.Errorf("check Docker availability: %w", err)
	}
	return nil
}

// CleanupEnabled parses the boolean environment variable controlling test cleanup.
func CleanupEnabled(envVar string) (bool, error) {
	value := os.Getenv(envVar)
	if value == "" {
		return false, nil
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s=%q: %w", envVar, value, err)
	}
	return enabled, nil
}

// AvailableTCPPort returns an available loopback TCP port.
func AvailableTCPPort() (port int, err error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve Kubernetes API port: %w", err)
	}
	defer func() {
		if closeErr := listener.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("release Kubernetes API port: %w", closeErr))
		}
	}()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("determine Kubernetes API port from %T", listener.Addr())
	}
	return address.Port, nil
}

// WriteBootstrapConfig writes the cluster bootstrap variables used by integration tests.
func WriteBootstrapConfig(path, clusterName string, apiPort int) error {
	content := fmt.Sprintf(`variables = {
  cluster_name    = %q
  k3d_api_port    = %q
  k3d_http_port   = ""
  k3d_https_port  = ""
}
`, clusterName, strconv.Itoa(apiPort))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write bootstrap config: %w", err)
	}
	return nil
}

// RunCommand runs a command with output streamed to the test process.
func RunCommand(ctx context.Context, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env

	var output strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &output)
	cmd.Stderr = io.MultiWriter(os.Stderr, &output)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s %s: %w\noutput:\n%s", name, strings.Join(args, " "), err, output.String())
	}
	return nil
}

// K3dClusterStatus reports whether the named cluster exists and is fully running.
func K3dClusterStatus(ctx context.Context, clusterName string) (exists, running bool, err error) {
	output, err := commandOutput(ctx, os.Environ(), "k3d", "cluster", "get", clusterName, "-o", "json")
	if err != nil {
		if isMissingClusterOutput(string(output)) {
			return false, false, nil
		}
		return false, false, err
	}

	var clusters []struct {
		ServersRunning int `json:"serversRunning"`
		ServersCount   int `json:"serversCount"`
		AgentsRunning  int `json:"agentsRunning"`
		AgentsCount    int `json:"agentsCount"`
	}
	if err := json.Unmarshal(output, &clusters); err != nil {
		return false, false, fmt.Errorf("parse k3d cluster status: %w", err)
	}
	if len(clusters) != 1 {
		return false, false, fmt.Errorf("expected one status entry for cluster %q, got %d", clusterName, len(clusters))
	}

	cluster := clusters[0]
	running = cluster.ServersCount > 0 &&
		cluster.ServersRunning == cluster.ServersCount &&
		cluster.AgentsRunning == cluster.AgentsCount
	return true, running, nil
}

// WriteK3dKubeconfig writes the named cluster's kubeconfig and selects it for the process.
func WriteK3dKubeconfig(ctx context.Context, clusterName, path string) error {
	kubeconfig, err := commandOutput(ctx, os.Environ(), "k3d", "kubeconfig", "get", clusterName)
	if err != nil {
		return fmt.Errorf("get kubeconfig for cluster %q: %w", clusterName, err)
	}
	if err := os.WriteFile(path, kubeconfig, 0o600); err != nil {
		return fmt.Errorf("write suite kubeconfig: %w", err)
	}
	if err := os.Setenv("KUBECONFIG", path); err != nil {
		return fmt.Errorf("set suite kubeconfig: %w", err)
	}
	return nil
}

// DeleteK3dClusterContext deletes a cluster and ignores an already-missing cluster.
func DeleteK3dClusterContext(ctx context.Context, clusterName string) error {
	output, err := commandOutput(ctx, os.Environ(), "k3d", "cluster", "delete", clusterName)
	if err != nil && !isMissingClusterOutput(string(output)) {
		return err
	}
	return nil
}

// AllocateTestNamespace creates a unique namespace and registers its cleanup.
func AllocateTestNamespace(t *testing.T, clusterName string, cleanupTimeout time.Duration) (string, *K8sClient) {
	t.Helper()

	suffix, err := randomHex(3)
	require.NoError(t, err)
	base := invalidDNSLabelChars.ReplaceAllString(strings.ToLower(t.Name()), "-")
	base = strings.Trim(base, "-")
	if len(base) > 45 {
		base = strings.TrimRight(base[:45], "-")
	}
	namespace := fmt.Sprintf("%s-%s", base, suffix)

	k8s := NewK8sClientOrFail(t)
	k8s.CreateNamespace(namespace, map[string]string{
		"uds-cli-test-run":  clusterName,
		"uds-cli-test-name": base,
	})
	t.Cleanup(func() {
		k8s.DeleteNamespaceAndWait(namespace, cleanupTimeout)
	})
	return namespace, k8s
}

// ZarfPackageStateSecretName returns the state secret name for a package override.
func ZarfPackageStateSecretName(packageName, namespace string) string {
	return fmt.Sprintf("zarf-package-%s-override-%s", packageName, namespace)
}

// PreparePodinfoBundle creates a podinfo bundle directory for a cluster test.
func PreparePodinfoBundle(t *testing.T, podinfoPackagePath, packageLabel, namespace string) string {
	t.Helper()

	dir := t.TempDir()
	valuesDir := filepath.Join(dir, "values")
	require.NoError(t, os.MkdirAll(valuesDir, 0o755))
	require.NoError(t, copyFile(
		TestDataPath("bundles/deploy/variables/values/podinfo.yaml"),
		filepath.Join(valuesDir, "podinfo.yaml"),
	))
	require.NoError(t, copyFile(
		TestDataPath("bundles/deploy/variables/defaults.uds.hcl"),
		filepath.Join(dir, "defaults.uds.hcl"),
	))

	packageName := filepath.Base(podinfoPackagePath)
	require.NoError(t, copyFile(podinfoPackagePath, filepath.Join(dir, packageName)))

	bundleHCL := fmt.Sprintf(`# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name    = "podinfo-cluster-test"
  version = "0.1.0"
}

package %q {
  source       = "./%s"
  namespace    = %q
  values_files = ["values/podinfo.yaml"]
  signature_verification { verify = false }
}
`, packageLabel, packageName, namespace)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bundle.uds.hcl"), []byte(bundleHCL), 0o600))
	return dir
}

// PrepareTwoPodinfoBundle creates a bundle containing two namespaced podinfo packages.
func PrepareTwoPodinfoBundle(t *testing.T, podinfoPackagePath, firstNamespace, secondNamespace string) string {
	t.Helper()

	dir := PreparePodinfoBundle(t, podinfoPackagePath, "pod_info_primary", firstNamespace)
	packageName := filepath.Base(podinfoPackagePath)
	bundleHCL := fmt.Sprintf(`# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name    = "k3d-core-init"
  version = "0.1.0"
}

package "pod_info_primary" {
  source       = "./%s"
  namespace    = %q
  values_files = ["values/podinfo.yaml"]
  signature_verification { verify = false }
}

package "pod_info_secondary" {
  source       = "./%s"
  namespace    = %q
  values_files = ["values/podinfo.yaml"]
  signature_verification { verify = false }
  depends_on   = [package.pod_info_primary]
}
`, packageName, firstNamespace, packageName, secondNamespace)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bundle.uds.hcl"), []byte(bundleHCL), 0o600))
	return dir
}

// RequireUDSCommand runs the CLI and fails the test when the command fails.
func RequireUDSCommand(t *testing.T, udsPath string, args ...string) []byte {
	t.Helper()
	output, err := runUDSCommand(t.Context(), t, udsPath, args...)
	require.NoError(t, err, "uds %s failed", strings.Join(args, " "))
	return output
}

// CreateBundleArtifact creates and locates a bundle artifact in bundleDir.
func CreateBundleArtifact(t *testing.T, udsPath, bundleDir string) string {
	t.Helper()
	RequireUDSCommand(t, udsPath, "bundle", "create", "--architecture", runtime.GOARCH, bundleDir)
	matches, err := filepath.Glob(filepath.Join(bundleDir, "uds-bundle-*.tar.zst"))
	require.NoError(t, err)
	require.Len(t, matches, 1, "expected exactly one bundle artifact in %s", bundleDir)
	return matches[0]
}

// RegisterBundleCleanup registers fallback removal of a deployed bundle.
func RegisterBundleCleanup(t *testing.T, udsPath, bundlePath string, cleanupTimeout time.Duration) func() {
	t.Helper()
	removed := false
	t.Cleanup(func() {
		if removed {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		cmd, err := bundleRemoveCommand(ctx, udsPath, bundlePath)
		if err != nil {
			t.Errorf("prepare cleanup for bundle %q: %v", bundlePath, err)
			return
		}
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("cleanup bundle %q: %v\n%s", bundlePath, err, output)
		}
	})
	return func() {
		removed = true
	}
}

// RemoveBundle removes a bundle with the CLI and returns its structured result.
func RemoveBundle(t *testing.T, udsPath, bundlePath string) bundle.RemoveResult {
	t.Helper()
	cmd, err := bundleRemoveCommand(t.Context(), udsPath, bundlePath)
	require.NoError(t, err)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		t.Logf("uds bundle remove output:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	require.NoError(t, err, "uds bundle remove %q failed", bundlePath)

	var result bundle.RemoveResult
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &result), "remove output should be valid JSON: %s", stdout.String())
	return result
}

func commandOutput(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("run %s %s: %w\noutput:\n%s", name, strings.Join(args, " "), err, output)
	}
	return output, nil
}

func isMissingClusterOutput(output string) bool {
	return strings.Contains(output, "No cluster(s) found") ||
		strings.Contains(output, "No nodes found for given cluster")
}

func randomHex(byteCount int) (string, error) {
	value := make([]byte, byteCount)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate random suffix: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func copyFile(source, destination string) (err error) {
	src, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", source, err)
	}
	defer func() {
		if closeErr := src.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close source file %q: %w", source, closeErr))
		}
	}()

	dst, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create destination file %q: %w", destination, err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return fmt.Errorf("copy %q to %q: %w", source, destination, err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("close destination file %q: %w", destination, err)
	}
	return nil
}

func runUDSCommand(ctx context.Context, t *testing.T, udsPath string, args ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.CommandContext(ctx, udsPath, args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("uds %s output:\n%s", strings.Join(args, " "), output)
	}
	return output, err
}

func bundleRemoveCommand(ctx context.Context, udsPath, bundlePath string) (*exec.Cmd, error) {
	info, err := os.Stat(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("inspect bundle path %q: %w", bundlePath, err)
	}

	target := bundlePath
	workingDir := ""
	if info.IsDir() {
		target = "."
		workingDir = bundlePath
	}

	cmd := exec.CommandContext(ctx, udsPath, "bundle", "remove", target, "-o", "json")
	cmd.Dir = workingDir
	cmd.Env = os.Environ()
	return cmd, nil
}
