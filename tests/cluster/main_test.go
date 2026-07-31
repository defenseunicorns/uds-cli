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
	"runtime"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

const (
	clusterSetupTimeout     = 20 * time.Minute
	clusterCleanupTimeout   = 5 * time.Minute
	namespaceCleanupTimeout = 3 * time.Minute
	sharedClusterName       = "uds-cli-integration"
	clusterCleanupEnvVar    = "UDS_TEST_CLUSTER_CLEANUP"
	zarfRegistryDeployment  = "zarf-docker-registry"
	zarfAgentDeployment     = "agent-hook"
	zarfStateSecret         = "zarf-state"
)

// suiteEnvironment owns the shared state and resources created by TestMain.
type suiteEnvironment struct {
	udsPath            string
	kubeconfigPath     string
	podinfoPackagePath string
	tempDir            string
	previousKubeconfig string
	hadKubeconfig      bool
	deleteCluster      bool
}

var testEnv *suiteEnvironment

func TestMain(m *testing.M) {
	os.Exit(runTestSuite(m))
}

func runTestSuite(m *testing.M) int {
	setupCtx, cancelSetup := context.WithTimeout(context.Background(), clusterSetupTimeout)
	defer cancelSetup()

	env, err := setupSuite(setupCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cluster integration setup failed: %v\n", err)
		return 1
	}
	testEnv = env

	testCode := m.Run()

	cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), clusterCleanupTimeout)
	defer cancelCleanup()
	if err := env.cleanup(cleanupCtx); err != nil {
		fmt.Fprintf(os.Stderr, "cluster integration cleanup failed: %v\n", err)
		if testCode == 0 {
			return 1
		}
	}

	return testCode
}

func setupSuite(ctx context.Context) (_ *suiteEnvironment, retErr error) {
	udsPath, err := testutil.ResolveUDSCLIPath()
	if err != nil {
		return nil, err
	}
	if err := testutil.CheckClusterPrerequisites(ctx); err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "uds-cli-cluster-integration")
	if err != nil {
		return nil, fmt.Errorf("create cluster integration temp directory: %w", err)
	}

	deleteCluster, err := testutil.CleanupEnabled(clusterCleanupEnvVar)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, err
	}

	env := &suiteEnvironment{
		udsPath:        udsPath,
		kubeconfigPath: filepath.Join(tempDir, "kubeconfig.yaml"),
		tempDir:        tempDir,
		deleteCluster:  deleteCluster,
	}
	env.previousKubeconfig, env.hadKubeconfig = os.LookupEnv("KUBECONFIG")

	defer func() {
		if retErr == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), clusterCleanupTimeout)
		defer cancel()
		retErr = errors.Join(retErr, env.cleanup(cleanupCtx))
	}()

	clusterExists, clusterRunning, err := testutil.K3dClusterStatus(ctx, sharedClusterName)
	if err != nil {
		return nil, fmt.Errorf("inspect k3d cluster %q: %w", sharedClusterName, err)
	}

	if clusterExists {
		if !clusterRunning {
			fmt.Fprintf(os.Stderr, "Starting retained cluster %q...\n", sharedClusterName)
			if err := testutil.RunCommand(ctx, os.Environ(), "k3d", "cluster", "start", sharedClusterName); err != nil {
				return nil, fmt.Errorf("start retained cluster %q: %w", sharedClusterName, err)
			}
		}
		fmt.Fprintf(os.Stderr, "Reusing shared cluster %q...\n", sharedClusterName)
		if err := testutil.WriteK3dKubeconfig(ctx, sharedClusterName, env.kubeconfigPath); err != nil {
			return nil, err
		}
		if err := waitForKubernetesAPI(ctx, env.kubeconfigPath); err != nil {
			return nil, err
		}
		initialized, err := zarfInitialized(ctx, env.kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("check Zarf initialization: %w", err)
		}
		if !initialized {
			fmt.Fprintln(os.Stderr, "Zarf init is missing or incomplete; installing it...")
			initBundle := testutil.TestDataPath("bundles/create/init-no-k3s")
			if err := testutil.RunCommand(ctx, os.Environ(), udsPath, "bundle", "deploy", initBundle); err != nil {
				return nil, fmt.Errorf("deploy Zarf init bundle: %w", err)
			}
		} else {
			fmt.Fprintln(os.Stderr, "Reusing existing Zarf init installation.")
		}
	} else {
		if err := os.Setenv("KUBECONFIG", env.kubeconfigPath); err != nil {
			return nil, fmt.Errorf("set suite kubeconfig: %w", err)
		}

		apiPort, err := testutil.AvailableTCPPort()
		if err != nil {
			return nil, err
		}
		configPath := filepath.Join(tempDir, "config.uds.hcl")
		if err := testutil.WriteBootstrapConfig(configPath, sharedClusterName, apiPort); err != nil {
			return nil, err
		}

		bootstrapBundle := testutil.TestDataPath("bundles/deploy/init")
		fmt.Fprintf(os.Stderr, "Creating shared cluster %q and installing Zarf init...\n", sharedClusterName)
		if err := testutil.RunCommand(ctx, os.Environ(), udsPath, "bundle", "deploy", bootstrapBundle, "--config", configPath); err != nil {
			return nil, fmt.Errorf("deploy cluster bootstrap bundle: %w", err)
		}
		if err := testutil.WriteK3dKubeconfig(ctx, sharedClusterName, env.kubeconfigPath); err != nil {
			return nil, err
		}
	}

	if err := waitForZarfReady(ctx, env.kubeconfigPath); err != nil {
		return nil, err
	}

	packageDir := filepath.Join(tempDir, "packages")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		return nil, fmt.Errorf("create suite package directory: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Building shared podinfo package...")
	if err := testutil.RunCommand(ctx, os.Environ(), udsPath,
		"zarf", "package", "create", testutil.TestDataPath("packages/podinfo"),
		"--output", packageDir,
		"--architecture", runtime.GOARCH,
		"--features", "values=true",
		"--confirm",
	); err != nil {
		return nil, fmt.Errorf("build shared podinfo package: %w", err)
	}

	env.podinfoPackagePath = filepath.Join(
		packageDir,
		fmt.Sprintf("zarf-package-podinfo-%s-0.1.0.tar.zst", runtime.GOARCH),
	)
	if _, err := os.Stat(env.podinfoPackagePath); err != nil {
		return nil, fmt.Errorf("locate shared podinfo package: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Shared cluster %q is ready.\n", sharedClusterName)
	return env, nil
}

func (e *suiteEnvironment) cleanup(ctx context.Context) error {
	var errs []error

	if e.deleteCluster {
		fmt.Fprintf(os.Stderr, "Deleting shared cluster %q...\n", sharedClusterName)
		if err := testutil.DeleteK3dClusterContext(ctx, sharedClusterName); err != nil {
			errs = append(errs, err)
		}
	} else {
		fmt.Fprintf(
			os.Stderr,
			"Retaining shared cluster %q for reuse. Delete it with: k3d cluster delete %s\n",
			sharedClusterName,
			sharedClusterName,
		)
	}

	if e.hadKubeconfig {
		if err := os.Setenv("KUBECONFIG", e.previousKubeconfig); err != nil {
			errs = append(errs, fmt.Errorf("restore KUBECONFIG: %w", err))
		}
	} else if err := os.Unsetenv("KUBECONFIG"); err != nil {
		errs = append(errs, fmt.Errorf("unset KUBECONFIG: %w", err))
	}

	if e.tempDir != "" {
		if err := os.RemoveAll(e.tempDir); err != nil {
			errs = append(errs, fmt.Errorf("remove suite temp directory: %w", err))
		}
	}

	return errors.Join(errs...)
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

func zarfInitialized(ctx context.Context, kubeconfigPath string) (bool, error) {
	client, err := kubernetesClient(kubeconfigPath)
	if err != nil {
		return false, err
	}
	_, err = client.CoreV1().Secrets("zarf").Get(ctx, zarfStateSecret, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get zarf/%s secret: %w", zarfStateSecret, err)
	}

	for _, name := range []string{zarfRegistryDeployment, zarfAgentDeployment} {
		_, err := client.AppsV1().Deployments("zarf").Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("get zarf/%s deployment: %w", name, err)
		}
	}
	return true, nil
}

func waitForKubernetesAPI(ctx context.Context, kubeconfigPath string) error {
	client, err := kubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	err = wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		_, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
		if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			return false, err
		}
		return err == nil, nil
	})
	if err != nil {
		return fmt.Errorf("wait for Kubernetes API to become ready: %w", err)
	}
	return nil
}

func waitForZarfReady(ctx context.Context, kubeconfigPath string) error {
	client, err := kubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	err = wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		for _, name := range []string{zarfRegistryDeployment, zarfAgentDeployment} {
			deployment, err := client.AppsV1().Deployments("zarf").Get(ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			}
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
			if deployment.Status.ObservedGeneration < deployment.Generation ||
				deployment.Status.AvailableReplicas < desiredReplicas {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("wait for Zarf deployments to become ready: %w", err)
	}
	return nil
}
