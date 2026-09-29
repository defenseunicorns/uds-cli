// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library && cluster_integration

package cluster_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	suppliedKubeconfigEnv  = "UDS_TEST_KUBECONFIG"
	zarfNamespace          = "zarf"
	zarfStateSecret        = "zarf-state"
	zarfRegistryDeployment = "zarf-docker-registry"
	zarfAgentDeployment    = "agent-hook"

	clusterCreateTimeout  = 20 * time.Minute
	clusterCleanupTimeout = 5 * time.Minute
	zarfReadyTimeout      = 5 * time.Minute
)

// suppliedCluster selects only the explicitly supplied test cluster. It never
// creates, switches, or deletes a cluster, and requires KUBECONFIG to already
// name the same explicit file.
func suppliedCluster(t *testing.T) *kubernetes.Clientset {
	t.Helper()
	kubeconfig := strings.TrimSpace(os.Getenv(suppliedKubeconfigEnv))
	require.NotEmpty(t, kubeconfig, "%s must name the supplied test kubeconfig", suppliedKubeconfigEnv)
	require.FileExists(t, kubeconfig, "supplied kubeconfig")
	require.Equal(t, kubeconfig, strings.TrimSpace(os.Getenv("KUBECONFIG")), "KUBECONFIG must equal %s", suppliedKubeconfigEnv)

	client := kubernetesClient(t, kubeconfig)
	requireZarfReady(t, client)
	return client
}

// createCluster creates an isolated k3d cluster and registers exact-name
// cleanup before invoking k3d. The default kubeconfig and current context are
// never modified.
func createCluster(t *testing.T) (name, kubeconfig string) {
	t.Helper()
	workDir := t.TempDir()
	name = "uds-library-" + randomClusterSuffix(t)
	kubeconfig = filepath.Join(workDir, "kubeconfig.yaml")

	before := clusterNames(t)
	require.NotContains(t, before, name, "generated cluster name already exists")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), clusterCleanupTimeout)
		defer cancel()
		clusters, err := listClusters(ctx)
		if err != nil {
			t.Errorf("list clusters during cleanup: %v", err)
			return
		}
		if _, exists := clusters[name]; !exists {
			return
		}
		if err := deleteCluster(ctx, name); err != nil {
			t.Errorf("delete owned cluster %q: %v", name, err)
		}
	})

	apiPort := freeAPIPort(t)
	ctx, cancel := context.WithTimeout(t.Context(), clusterCreateTimeout)
	defer cancel()
	_, err := runK3d(ctx,
		"cluster", "create", name,
		"--servers", "1",
		"--agents", "0",
		"--api-port", "127.0.0.1:"+strconv.Itoa(apiPort),
		"--kubeconfig-update-default=false",
		"--kubeconfig-switch-context=false",
		"--timeout", clusterCreateTimeout.String(),
		"--wait",
	)
	require.NoError(t, err)
	config, err := kubeconfigForCluster(ctx, name)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(kubeconfig, config, 0o600))
	require.FileExists(t, kubeconfig)
	return name, kubeconfig
}

func kubernetesClient(t *testing.T, kubeconfig string) *kubernetes.Clientset {
	t.Helper()
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err, "load kubeconfig %q", kubeconfig)
	client, err := kubernetes.NewForConfig(config)
	require.NoError(t, err, "create Kubernetes client")
	return client
}

func requireZarfReady(t *testing.T, client *kubernetes.Clientset) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), zarfReadyTimeout)
	defer cancel()
	var lastErr error
	err := wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		if _, err := client.CoreV1().Secrets(zarfNamespace).Get(ctx, zarfStateSecret, metav1.GetOptions{}); err != nil {
			lastErr = err
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return false, err
			}
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, nil
		}

		for _, name := range []string{zarfRegistryDeployment, zarfAgentDeployment} {
			deployment, err := client.AppsV1().Deployments(zarfNamespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				lastErr = err
				if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
					return false, err
				}
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				return false, nil
			}
			desired := int32(1)
			if deployment.Spec.Replicas != nil {
				desired = *deployment.Spec.Replicas
			}
			if desired <= 0 {
				return false, fmt.Errorf("deployment %q has invalid desired replicas %d", name, desired)
			}
			if deployment.Status.ObservedGeneration < deployment.Generation || deployment.Status.AvailableReplicas < desired {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil && lastErr != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) {
		err = lastErr
	}
	require.NoError(t, err, "Zarf state, registry, and agent readiness")
}

func initBundlePath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve test source path")
	candidate := filepath.Join(filepath.Dir(file), "..", "..", "test_data", "bundles", "create", "init-no-k3s", "bundle.uds.hcl")
	require.FileExists(t, candidate)
	return candidate
}

func freeAPIPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "allocate Kubernetes API port")
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return port
}

func clusterNames(t *testing.T) map[string]struct{} {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	names, err := listClusters(ctx)
	require.NoError(t, err)
	return names
}

func listClusters(ctx context.Context) (map[string]struct{}, error) {
	output, err := runK3d(ctx, "cluster", "list", "--no-headers")
	if err != nil {
		return nil, err
	}
	names := map[string]struct{}{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			names[fields[0]] = struct{}{}
		}
	}
	return names, nil
}

func kubeconfigForCluster(ctx context.Context, name string) ([]byte, error) {
	command := exec.CommandContext(ctx, "k3d", "kubeconfig", "get", name)
	command.Env = append(os.Environ(), "NO_COLOR=1")
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("k3d kubeconfig get %q: %w", name, err)
	}
	return output, nil
}

func deleteCluster(ctx context.Context, name string) error {
	_, err := runK3d(ctx, "cluster", "delete", name)
	if err != nil {
		return fmt.Errorf("delete cluster %q: %w", name, err)
	}
	return nil
}

func runK3d(ctx context.Context, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "k3d", args...)
	command.Env = append(os.Environ(), "NO_COLOR=1")
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("k3d %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func randomClusterSuffix(t *testing.T) string {
	t.Helper()
	bytes := make([]byte, 8)
	_, err := rand.Read(bytes)
	require.NoError(t, err, "generate unique cluster name")
	return hex.EncodeToString(bytes)
}
