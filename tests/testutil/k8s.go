// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type K8sClient struct {
	*kubernetes.Clientset
	t *testing.T
}

func NewK8sClientOrSkip(t *testing.T) *K8sClient {
	t.Helper()
	return newK8sClient(t, false)
}

func NewK8sClientOrFail(t *testing.T) *K8sClient {
	t.Helper()
	return newK8sClient(t, true)
}

func newK8sClient(t *testing.T, failIfUnavailable bool) *K8sClient {
	t.Helper()

	config, err := rest.InClusterConfig()
	if err != nil {
		config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			&clientcmd.ConfigOverrides{},
		).ClientConfig()
		if err != nil {
			handleClusterUnavailable(t, failIfUnavailable, err)
			return nil
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("failed to create Kubernetes client: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err = clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		handleClusterUnavailable(t, failIfUnavailable, err)
		return nil
	}

	return &K8sClient{Clientset: clientset, t: t}
}

func handleClusterUnavailable(t *testing.T, failIfUnavailable bool, err error) {
	t.Helper()
	if failIfUnavailable {
		t.Fatalf("Kubernetes cluster not available: %v", err)
	}
	t.Skipf("Kubernetes cluster not available: %v", err)
}

func (c *K8sClient) AssertNamespaceExists(namespace string) {
	c.t.Helper()
	ns, err := c.CoreV1().Namespaces().Get(c.t.Context(), namespace, metav1.GetOptions{})
	require.NoError(c.t, err, "namespace %q should exist", namespace)
	require.NotNil(c.t, ns)
}

func (c *K8sClient) AssertNamespaceNotExists(namespace string) {
	c.t.Helper()
	_, err := c.CoreV1().Namespaces().Get(c.t.Context(), namespace, metav1.GetOptions{})
	require.True(c.t, errors.IsNotFound(err), "namespace %q should not exist", namespace)
}

// CreateNamespace creates a namespace with the provided labels.
func (c *K8sClient) CreateNamespace(name string, labels map[string]string) {
	c.t.Helper()
	_, err := c.CoreV1().Namespaces().Create(c.t.Context(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}, metav1.CreateOptions{})
	require.NoError(c.t, err, "create namespace %q", name)
}

// DeleteNamespaceAndWait deletes a namespace and waits until it no longer exists.
func (c *K8sClient) DeleteNamespaceAndWait(name string, timeout time.Duration) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := c.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		require.NoError(c.t, err, "delete namespace %q", name)
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		_, err := c.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return
		}
		select {
		case <-ctx.Done():
			c.t.Errorf("timeout waiting for namespace %q to be deleted", name)
			return
		case <-ticker.C:
		}
	}
}

func (c *K8sClient) AssertSecretExists(namespace, name string) {
	c.t.Helper()
	secret, err := c.CoreV1().Secrets(namespace).Get(c.t.Context(), name, metav1.GetOptions{})
	require.NoError(c.t, err, "secret %q in namespace %q should exist", name, namespace)
	require.NotNil(c.t, secret)
}

func (c *K8sClient) AssertSecretNotExists(namespace, name string) {
	c.t.Helper()
	_, err := c.CoreV1().Secrets(namespace).Get(c.t.Context(), name, metav1.GetOptions{})
	require.True(c.t, errors.IsNotFound(err), "secret %q in namespace %q should not exist", name, namespace)
}

func (c *K8sClient) AssertDeploymentExists(namespace, name string) {
	c.t.Helper()
	deployment, err := c.AppsV1().Deployments(namespace).Get(c.t.Context(), name, metav1.GetOptions{})
	require.NoError(c.t, err, "deployment %q in namespace %q should exist", name, namespace)
	require.NotNil(c.t, deployment)
}

func (c *K8sClient) AssertDeploymentNotExists(namespace, name string) {
	c.t.Helper()
	_, err := c.AppsV1().Deployments(namespace).Get(c.t.Context(), name, metav1.GetOptions{})
	require.True(c.t, errors.IsNotFound(err), "deployment %q in namespace %q should not exist", name, namespace)
}

func (c *K8sClient) GetNamespace(name string) *corev1.Namespace {
	c.t.Helper()
	ns, err := c.CoreV1().Namespaces().Get(c.t.Context(), name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	require.NoError(c.t, err, "failed to get namespace %q", name)
	return ns
}

func (c *K8sClient) GetSecret(namespace, name string) *corev1.Secret {
	c.t.Helper()
	secret, err := c.CoreV1().Secrets(namespace).Get(c.t.Context(), name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	require.NoError(c.t, err, "failed to get secret %q in namespace %q", name, namespace)
	return secret
}

func (c *K8sClient) WaitForNamespace(namespace string, timeout time.Duration) {
	c.t.Helper()
	waitFor(c.t, timeout, fmt.Sprintf("namespace %q to exist", namespace), func(ctx context.Context) bool {
		_, err := c.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		return err == nil
	})
}

func (c *K8sClient) AssertDeploymentPodAnnotation(namespace, name, key, value string) {
	c.t.Helper()
	deployment, err := c.AppsV1().Deployments(namespace).Get(c.t.Context(), name, metav1.GetOptions{})
	require.NoError(c.t, err, "deployment %q in namespace %q should exist", name, namespace)
	require.Equal(c.t, value, deployment.Spec.Template.Annotations[key],
		"pod template annotation %q should be %q on deployment %q", key, value, name)
}

func (c *K8sClient) AssertDeploymentPodToleration(namespace, name, key string, operator corev1.TolerationOperator, effect corev1.TaintEffect) {
	c.t.Helper()
	deployment, err := c.AppsV1().Deployments(namespace).Get(c.t.Context(), name, metav1.GetOptions{})
	require.NoError(c.t, err, "deployment %q in namespace %q should exist", name, namespace)
	for _, toleration := range deployment.Spec.Template.Spec.Tolerations {
		if toleration.Key == key && toleration.Operator == operator && toleration.Effect == effect {
			return
		}
	}
	c.t.Fatalf("deployment %q missing toleration key=%s operator=%s effect=%s", name, key, operator, effect)
}

func (c *K8sClient) AssertDeploymentReplicas(namespace, name string, expected int32) {
	c.t.Helper()
	deployment, err := c.AppsV1().Deployments(namespace).Get(c.t.Context(), name, metav1.GetOptions{})
	require.NoError(c.t, err, "deployment %q in namespace %q should exist", name, namespace)
	require.NotNil(c.t, deployment.Spec.Replicas, "deployment %q spec.replicas should not be nil", name)
	require.Equal(c.t, expected, *deployment.Spec.Replicas,
		"deployment %q in namespace %q should have %d replica(s)", name, namespace, expected)
}

func (c *K8sClient) AssertStatefulSetReplicas(namespace, name string, expected int32) {
	c.t.Helper()
	statefulSet, err := c.AppsV1().StatefulSets(namespace).Get(c.t.Context(), name, metav1.GetOptions{})
	require.NoError(c.t, err, "statefulset %q in namespace %q should exist", name, namespace)
	require.NotNil(c.t, statefulSet.Spec.Replicas, "statefulset %q in namespace %q should declare replicas", name, namespace)
	require.Equal(c.t, expected, *statefulSet.Spec.Replicas, "statefulset %q in namespace %q should have %d replicas", name, namespace, expected)
}

func (c *K8sClient) AssertDeploymentContainerRequests(namespace, deploymentName, containerName, expectedCPU, expectedMemory string) {
	c.t.Helper()
	deployment, err := c.AppsV1().Deployments(namespace).Get(c.t.Context(), deploymentName, metav1.GetOptions{})
	require.NoError(c.t, err, "deployment %q in namespace %q should exist", deploymentName, namespace)

	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name != containerName {
			continue
		}
		require.Equal(c.t, expectedCPU, container.Resources.Requests.Cpu().String(), "deployment %q container %q should request cpu %q", deploymentName, containerName, expectedCPU)
		require.Equal(c.t, expectedMemory, container.Resources.Requests.Memory().String(), "deployment %q container %q should request memory %q", deploymentName, containerName, expectedMemory)
		return
	}

	c.t.Fatalf("deployment %q in namespace %q missing container %q", deploymentName, namespace, containerName)
}

func (c *K8sClient) AssertServiceNotExists(namespace, name string) {
	c.t.Helper()
	_, err := c.CoreV1().Services(namespace).Get(c.t.Context(), name, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		require.NoError(c.t, err, "unexpected error checking service %q in namespace %q", name, namespace)
	}
	require.True(c.t, errors.IsNotFound(err), "service %q in namespace %q should not exist", name, namespace)
}

func (c *K8sClient) WaitForDeploymentReady(namespace, name string, timeout time.Duration) {
	c.t.Helper()
	waitFor(c.t, timeout, fmt.Sprintf("deployment %q in namespace %q to be ready", name, namespace), func(ctx context.Context) bool {
		deployment, err := c.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false
		}
		return deploymentReady(deployment)
	})
}

func (c *K8sClient) WaitForStatefulSetReady(namespace, name string, timeout time.Duration) {
	c.t.Helper()
	waitFor(c.t, timeout, fmt.Sprintf("statefulset %q in namespace %q to be ready", name, namespace), func(ctx context.Context) bool {
		statefulSet, err := c.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false
		}
		return statefulSetReady(statefulSet)
	})
}

func (c *K8sClient) WaitForReadyPodBySelector(namespace, labelSelector string, timeout time.Duration) {
	c.t.Helper()
	waitFor(c.t, timeout, fmt.Sprintf("ready pod with selector %q in namespace %q", labelSelector, namespace), func(ctx context.Context) bool {
		pods, err := c.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			return false
		}
		for _, pod := range pods.Items {
			if podReady(&pod) {
				return true
			}
		}
		return false
	})
}

func waitFor(t *testing.T, timeout time.Duration, description string, ready func(context.Context) bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for %s", description)
		case <-ticker.C:
			if ready(ctx) {
				return
			}
		}
	}
}

func deploymentReady(deployment *appsv1.Deployment) bool {
	if deployment.Spec.Replicas == nil {
		return false
	}
	return deployment.Status.ReadyReplicas == *deployment.Spec.Replicas
}

func statefulSetReady(statefulSet *appsv1.StatefulSet) bool {
	if statefulSet.Spec.Replicas == nil {
		return false
	}
	return statefulSet.Status.ReadyReplicas == *statefulSet.Spec.Replicas
}

func podReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
