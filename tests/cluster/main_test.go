// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cluster_integration

package cluster_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
	"github.com/zarf-dev/zarf/src/pkg/feature"
	"github.com/zarf-dev/zarf/src/pkg/packager"
)

const (
	clusterSetupTimeout     = 20 * time.Minute
	namespaceCleanupTimeout = 3 * time.Minute
	suppliedKubeconfigEnv   = "UDS_TEST_KUBECONFIG"
	sharedClusterName       = "uds-cli-integration"
	zarfRegistryDeployment  = "zarf-docker-registry"
	zarfAgentDeployment     = "agent-hook"
	zarfStateSecret         = "zarf-state"
)

type suiteEnvironment struct {
	kubeconfigPath     string
	podinfoPackagePath string
	monitorPackagePath string
	tempDir            string
}

var testEnv *suiteEnvironment

func TestMain(m *testing.M) {
	os.Exit(runTestSuite(m))
}

func runTestSuite(m *testing.M) int {
	setupCtx, cancelSetup := context.WithTimeout(context.Background(), clusterSetupTimeout)
	env, err := setupSuite(setupCtx)
	cancelSetup()
	if err != nil {
		fmt.Fprintf(os.Stderr, "supplied cluster integration setup failed: %v\n", err)
		return 1
	}
	testEnv = env

	testCode := m.Run()
	if err := env.cleanup(); err != nil {
		fmt.Fprintf(os.Stderr, "cluster integration cleanup failed: %v\n", err)
		if testCode == 0 {
			return 1
		}
	}
	return testCode
}

func setupSuite(ctx context.Context) (_ *suiteEnvironment, retErr error) {
	kubeconfigPath := strings.TrimSpace(os.Getenv(suppliedKubeconfigEnv))
	if kubeconfigPath == "" {
		return nil, fmt.Errorf("%s must name the supplied test kubeconfig", suppliedKubeconfigEnv)
	}
	if strings.TrimSpace(os.Getenv("KUBECONFIG")) != kubeconfigPath {
		return nil, fmt.Errorf("KUBECONFIG must equal %s", suppliedKubeconfigEnv)
	}
	info, err := os.Stat(kubeconfigPath) //nolint:gosec // kubeconfig is the explicitly required test path
	if err != nil {
		return nil, fmt.Errorf("inspect supplied kubeconfig %q: %w", kubeconfigPath, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("supplied kubeconfig %q is a directory", kubeconfigPath)
	}

	if err := waitForZarfReady(ctx, kubeconfigPath); err != nil {
		return nil, fmt.Errorf("verify supplied Zarf readiness: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "uds-cli-cluster-integration")
	if err != nil {
		return nil, fmt.Errorf("create cluster integration temp directory: %w", err)
	}
	env := &suiteEnvironment{
		kubeconfigPath: kubeconfigPath,
		tempDir:        tempDir,
	}
	defer func() {
		if retErr != nil {
			if cleanupErr := env.cleanup(); cleanupErr != nil {
				retErr = errors.Join(retErr, cleanupErr)
			}
		}
	}()

	packageDir := filepath.Join(tempDir, "packages")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		return nil, fmt.Errorf("create suite package directory: %w", err)
	}
	if err := feature.Set([]feature.Feature{{Name: feature.Values, Enabled: true, Stage: feature.Alpha}}); err != nil {
		return nil, fmt.Errorf("enable Zarf values: %w", err)
	}
	env.podinfoPackagePath, err = packager.Create(ctx, testutil.TestDataPath("packages/podinfo"), packageDir, packager.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("build shared podinfo package: %w", err)
	}
	env.monitorPackagePath, err = packager.Create(ctx, testutil.TestDataPath("packages/operator-monitor"), packageDir, packager.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("build operator monitor test package: %w", err)
	}

	return env, nil
}

func (e *suiteEnvironment) cleanup() error {
	if e.tempDir == "" {
		return nil
	}
	return os.RemoveAll(e.tempDir)
}

func kubernetesClient(kubeconfigPath string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes client: %w", err)
	}
	return client, nil
}

func waitForZarfReady(ctx context.Context, kubeconfigPath string) error {
	client, err := kubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	err = wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		if _, err := client.CoreV1().Secrets("zarf").Get(ctx, zarfStateSecret, metav1.GetOptions{}); err != nil {
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return false, err
			}
			return false, nil
		}
		for _, name := range []string{zarfRegistryDeployment, zarfAgentDeployment} {
			deployment, err := client.AppsV1().Deployments("zarf").Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
					return false, fmt.Errorf("get zarf/%s deployment: %w", name, err)
				}
				return false, nil
			}
			desiredReplicas := int32(1)
			if deployment.Spec.Replicas != nil {
				desiredReplicas = *deployment.Spec.Replicas
			}
			if desiredReplicas <= 0 {
				return false, fmt.Errorf("deployment %q has invalid desired replicas %d", name, desiredReplicas)
			}
			if deployment.Status.ObservedGeneration < deployment.Generation ||
				deployment.Status.AvailableReplicas < desiredReplicas {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("wait for Zarf state, registry, and agent readiness: %w", err)
	}
	return nil
}
