// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const namespaceCleanupTimeout = 2 * time.Minute

// Kubernetes provides assertions scoped to one suite kubeconfig.
type Kubernetes struct {
	t      *testing.T
	client kubernetes.Interface
}

// NewKubernetes creates a Kubernetes test client from an explicit kubeconfig.
func NewKubernetes(t *testing.T, kubeconfigPath string) *Kubernetes {
	t.Helper()

	client, err := kubernetesClient(kubeconfigPath)
	if err != nil {
		t.Fatalf("create Kubernetes client: %v", err)
	}
	return &Kubernetes{t: t, client: client}
}

// AssertNamespaceExists fails the test when name is not an existing namespace.
func (k *Kubernetes) AssertNamespaceExists(name string) {
	k.t.Helper()

	_, err := k.client.CoreV1().Namespaces().Get(k.t.Context(), name, metav1.GetOptions{})
	if err != nil {
		k.t.Errorf("expected namespace %q to exist: %v", name, err)
	}
}

// AssertNamespaceDoesNotExist fails the test when name is still an existing namespace.
func (k *Kubernetes) AssertNamespaceDoesNotExist(name string) {
	k.t.Helper()

	_, err := k.client.CoreV1().Namespaces().Get(k.t.Context(), name, metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		k.t.Errorf("expected namespace %q not to exist, got: %v", name, err)
	}
}

// AssertDeploymentExists fails the test when the deployment is absent.
func (k *Kubernetes) AssertDeploymentExists(namespace, name string) {
	k.t.Helper()

	_, err := k.client.AppsV1().Deployments(namespace).Get(k.t.Context(), name, metav1.GetOptions{})
	if err != nil {
		k.t.Errorf("expected deployment %s/%s to exist: %v", namespace, name, err)
	}
}

// AssertDeploymentDoesNotExist fails the test when the deployment exists.
func (k *Kubernetes) AssertDeploymentDoesNotExist(namespace, name string) {
	k.t.Helper()

	_, err := k.client.AppsV1().Deployments(namespace).Get(k.t.Context(), name, metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		k.t.Errorf("expected deployment %s/%s not to exist, got: %v", namespace, name, err)
	}
}

// DeploymentImage returns the first container image from a deployment.
func (k *Kubernetes) DeploymentImage(namespace, name string) string {
	k.t.Helper()

	deployment, err := k.client.AppsV1().Deployments(namespace).Get(k.t.Context(), name, metav1.GetOptions{})
	if err != nil {
		k.t.Fatalf("get deployment %s/%s: %v", namespace, name, err)
	}
	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		k.t.Fatalf("deployment %s/%s has no containers", namespace, name)
	}
	return deployment.Spec.Template.Spec.Containers[0].Image
}

// Deployment returns a namespace-scoped deployment for detailed assertions.
func (k *Kubernetes) Deployment(namespace, name string) *appsv1.Deployment {
	k.t.Helper()

	deployment, err := k.client.AppsV1().Deployments(namespace).Get(k.t.Context(), name, metav1.GetOptions{})
	if err != nil {
		k.t.Fatalf("get deployment %s/%s: %v", namespace, name, err)
	}
	return deployment
}

// Secret returns a namespace-scoped secret for detailed assertions.
func (k *Kubernetes) Secret(namespace, name string) *corev1.Secret {
	k.t.Helper()

	secret, err := k.client.CoreV1().Secrets(namespace).Get(k.t.Context(), name, metav1.GetOptions{})
	if err != nil {
		k.t.Fatalf("get secret %s/%s: %v", namespace, name, err)
	}
	return secret
}

// Ingress returns a namespace-scoped ingress for detailed assertions.
func (k *Kubernetes) Ingress(namespace, name string) *networkingv1.Ingress {
	k.t.Helper()

	ingress, err := k.client.NetworkingV1().Ingresses(namespace).Get(k.t.Context(), name, metav1.GetOptions{})
	if err != nil {
		k.t.Fatalf("get ingress %s/%s: %v", namespace, name, err)
	}
	return ingress
}

// WaitForDeploymentReady waits until all desired replicas are available.
func (k *Kubernetes) WaitForDeploymentReady(namespace, name string, timeout time.Duration) {
	k.t.Helper()

	err := wait.PollUntilContextTimeout(k.t.Context(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		deployment, err := k.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return deploymentReady(deployment), nil
	})
	if err != nil {
		k.t.Fatalf("wait for deployment %s/%s: %v", namespace, name, err)
	}
}

// WaitForStatefulSetReady waits until all desired replicas are ready.
func (k *Kubernetes) WaitForStatefulSetReady(namespace, name string, timeout time.Duration) {
	k.t.Helper()

	err := wait.PollUntilContextTimeout(k.t.Context(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		statefulSet, err := k.client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		desired := int32(1)
		if statefulSet.Spec.Replicas != nil {
			desired = *statefulSet.Spec.Replicas
		}
		return statefulSet.Status.ObservedGeneration >= statefulSet.Generation &&
			statefulSet.Status.ReadyReplicas == desired, nil
	})
	if err != nil {
		k.t.Fatalf("wait for statefulset %s/%s: %v", namespace, name, err)
	}
}

// WaitForDeploymentPodsReady waits for every pod selected by a deployment to be ready.
func (k *Kubernetes) WaitForDeploymentPodsReady(namespace, name string, timeout time.Duration) {
	k.t.Helper()

	deployment, err := k.client.AppsV1().Deployments(namespace).Get(k.t.Context(), name, metav1.GetOptions{})
	if err != nil {
		k.t.Fatalf("get deployment %s/%s pod selector: %v", namespace, name, err)
	}
	k.waitForPodsReady(namespace, deployment.Spec.Selector, timeout, "deployment "+name)
}

// WaitForStatefulSetPodsReady waits for every pod selected by a statefulset to be ready.
func (k *Kubernetes) WaitForStatefulSetPodsReady(namespace, name string, timeout time.Duration) {
	k.t.Helper()

	statefulSet, err := k.client.AppsV1().StatefulSets(namespace).Get(k.t.Context(), name, metav1.GetOptions{})
	if err != nil {
		k.t.Fatalf("get statefulset %s/%s pod selector: %v", namespace, name, err)
	}
	k.waitForPodsReady(namespace, statefulSet.Spec.Selector, timeout, "statefulset "+name)
}

// AssertDeploymentContainerRequests verifies every workload container requests CPU and memory.
func (k *Kubernetes) AssertDeploymentContainerRequests(namespace, name string) {
	k.t.Helper()

	deployment, err := k.client.AppsV1().Deployments(namespace).Get(k.t.Context(), name, metav1.GetOptions{})
	if err != nil {
		k.t.Errorf("get deployment %s/%s resource requests: %v", namespace, name, err)
		return
	}
	k.assertContainerRequests("deployment", namespace, name, deployment.Spec.Template.Spec.Containers)
}

// AssertStatefulSetContainerRequests verifies every workload container requests CPU and memory.
func (k *Kubernetes) AssertStatefulSetContainerRequests(namespace, name string) {
	k.t.Helper()

	statefulSet, err := k.client.AppsV1().StatefulSets(namespace).Get(k.t.Context(), name, metav1.GetOptions{})
	if err != nil {
		k.t.Errorf("get statefulset %s/%s resource requests: %v", namespace, name, err)
		return
	}
	k.assertContainerRequests("statefulset", namespace, name, statefulSet.Spec.Template.Spec.Containers)
}

func (k *Kubernetes) waitForPodsReady(namespace string, selector *metav1.LabelSelector, timeout time.Duration, owner string) {
	k.t.Helper()

	labelSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		k.t.Fatalf("build %s pod selector in namespace %s: %v", owner, namespace, err)
	}
	err = wait.PollUntilContextTimeout(k.t.Context(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pods, err := k.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector.String()})
		if err != nil {
			return false, err
		}
		if len(pods.Items) == 0 {
			return false, nil
		}
		for index := range pods.Items {
			if !podReady(&pods.Items[index]) {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		k.t.Fatalf("wait for %s pods in namespace %s: %v", owner, namespace, err)
	}
}

func (k *Kubernetes) assertContainerRequests(kind, namespace, name string, containers []corev1.Container) {
	k.t.Helper()

	if len(containers) == 0 {
		k.t.Errorf("%s %s/%s has no containers", kind, namespace, name)
		return
	}
	for _, container := range containers {
		if container.Resources.Requests.Cpu().IsZero() {
			k.t.Errorf("%s %s/%s container %q has no CPU request", kind, namespace, name, container.Name)
		}
		if container.Resources.Requests.Memory().IsZero() {
			k.t.Errorf("%s %s/%s container %q has no memory request", kind, namespace, name, container.Name)
		}
	}
}

func kubernetesClient(kubeconfigPath string) (kubernetes.Interface, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig %q: %w", kubeconfigPath, err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("build client from kubeconfig %q: %w", kubeconfigPath, err)
	}
	return client, nil
}

func deploymentReady(deployment *appsv1.Deployment) bool {
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	return deployment.Status.ObservedGeneration >= deployment.Generation &&
		deployment.Status.AvailableReplicas == desired
}

func podReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning || len(pod.Status.ContainerStatuses) == 0 {
		return false
	}
	for _, status := range pod.Status.ContainerStatuses {
		if !status.Ready {
			return false
		}
	}
	return true
}

func deleteNamespaceAndWait(ctx context.Context, client kubernetes.Interface, name string) error {
	propagation := metav1.DeletePropagationBackground
	err := client.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &propagation})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete namespace %q: %w", name, err)
	}

	err = wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		_, err := client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			return true, nil
		case err != nil:
			return false, err
		default:
			return false, nil
		}
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("timed out waiting for namespace %q deletion: %w", name, err)
		}
		return fmt.Errorf("wait for namespace %q deletion: %w", name, err)
	}
	return nil
}
