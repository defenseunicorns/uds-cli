// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package bundle_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// K8sClient wraps kubernetes.Clientset with helper methods for testing.
type K8sClient struct {
	*kubernetes.Clientset
	t *testing.T
}

// NewK8sClient creates a new K8sClient using the default kubeconfig.
// It skips the test if the cluster is not reachable.
func NewK8sClient(t *testing.T) *K8sClient {
	t.Helper()

	// Build config from default kubeconfig location
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		t.Skipf("Kubernetes cluster not available: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// Verify cluster is reachable
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		t.Skipf("Kubernetes cluster not reachable: %v", err)
	}

	return &K8sClient{Clientset: clientset, t: t}
}

// AssertNamespaceExists verifies that a namespace exists.
func (c *K8sClient) AssertNamespaceExists(namespace string) {
	c.t.Helper()
	ctx := context.Background()

	ns, err := c.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	require.NoError(c.t, err, "namespace %q should exist", namespace)
	require.NotNil(c.t, ns)
	c.t.Logf("✓ namespace %q exists", namespace)
}

// AssertNamespaceNotExists verifies that a namespace does not exist.
func (c *K8sClient) AssertNamespaceNotExists(namespace string) {
	c.t.Helper()
	ctx := context.Background()

	_, err := c.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	require.True(c.t, errors.IsNotFound(err), "namespace %q should not exist", namespace)
	c.t.Logf("✓ namespace %q does not exist", namespace)
}

// AssertSecretExists verifies that a secret exists in the given namespace.
func (c *K8sClient) AssertSecretExists(namespace, name string) {
	c.t.Helper()
	ctx := context.Background()

	secret, err := c.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	require.NoError(c.t, err, "secret %q in namespace %q should exist", name, namespace)
	require.NotNil(c.t, secret)
	c.t.Logf("✓ secret %q in namespace %q exists", name, namespace)
}

// AssertSecretNotExists verifies that a secret does not exist in the given namespace.
func (c *K8sClient) AssertSecretNotExists(namespace, name string) {
	c.t.Helper()
	ctx := context.Background()

	_, err := c.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	require.True(c.t, errors.IsNotFound(err), "secret %q in namespace %q should not exist", name, namespace)
	c.t.Logf("✓ secret %q in namespace %q does not exist", name, namespace)
}

// AssertDeploymentExists verifies that a deployment exists in the given namespace.
func (c *K8sClient) AssertDeploymentExists(namespace, name string) {
	c.t.Helper()
	ctx := context.Background()

	deployment, err := c.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	require.NoError(c.t, err, "deployment %q in namespace %q should exist", name, namespace)
	require.NotNil(c.t, deployment)
	c.t.Logf("✓ deployment %q in namespace %q exists", name, namespace)
}

// AssertDeploymentNotExists verifies that a deployment does not exist in the given namespace.
func (c *K8sClient) AssertDeploymentNotExists(namespace, name string) {
	c.t.Helper()
	ctx := context.Background()

	_, err := c.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	require.True(c.t, errors.IsNotFound(err), "deployment %q in namespace %q should not exist", name, namespace)
	c.t.Logf("✓ deployment %q in namespace %q does not exist", name, namespace)
}

// GetNamespace returns a namespace by name, or nil if it doesn't exist.
func (c *K8sClient) GetNamespace(name string) *corev1.Namespace {
	c.t.Helper()
	ctx := context.Background()

	ns, err := c.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	require.NoError(c.t, err, "failed to get namespace %q", name)
	return ns
}

// GetSecret returns a secret by namespace and name, or nil if it doesn't exist.
func (c *K8sClient) GetSecret(namespace, name string) *corev1.Secret {
	c.t.Helper()
	ctx := context.Background()

	secret, err := c.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	require.NoError(c.t, err, "failed to get secret %q in namespace %q", name, namespace)
	return secret
}

// WaitForNamespace waits for a namespace to exist, with a timeout.
func (c *K8sClient) WaitForNamespace(namespace string, timeout time.Duration) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.t.Fatalf("timeout waiting for namespace %q to exist", namespace)
		case <-ticker.C:
			_, err := c.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
			if err == nil {
				c.t.Logf("✓ namespace %q exists", namespace)
				return
			}
			if !errors.IsNotFound(err) {
				c.t.Logf("error checking namespace %q: %v", namespace, err)
			}
		}
	}
}

// WaitForDeploymentReady waits for a deployment to have all replicas ready.
func (c *K8sClient) WaitForDeploymentReady(namespace, name string, timeout time.Duration) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.t.Fatalf("timeout waiting for deployment %q in namespace %q to be ready", name, namespace)
		case <-ticker.C:
			deployment, err := c.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					continue
				}
				c.t.Logf("error checking deployment %q: %v", name, err)
				continue
			}

			if deployment.Status.ReadyReplicas == *deployment.Spec.Replicas {
				c.t.Logf("✓ deployment %q in namespace %q is ready (%d/%d replicas)",
					name, namespace, deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
				return
			}
			c.t.Logf("waiting for deployment %q: %d/%d replicas ready",
				name, deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
		}
	}
}
