// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cluster_integration

package cluster_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

const (
	peprSystemNamespace      = "pepr-system"
	monitorResourceNamespace = "uds-cli-monitor"
	monitorPodTimeout        = 5 * time.Minute
)

func TestOperatorMonitor(t *testing.T) {
	k8s := testutil.NewK8sClientOrFail(t)
	registerMonitorNamespaceCleanup(t, k8s)
	deployMonitorPackage(t)
	waitForMonitorPod(t, k8s, "uds-cli-monitor-admission")
	waitForMonitorPod(t, k8s, "uds-cli-monitor-watcher")

	t.Run("streams operator and policy events", func(t *testing.T) {
		output := testutil.RequireUDSCommand(t, testEnv.udsPath,
			"core", "operator", "monitor",
			"--namespace", monitorResourceNamespace,
			"--no-color",
		)

		assert.Contains(t, string(output), "ALLOWED  resource="+monitorResourceNamespace+"/allowed-pod")
		assert.Contains(t, string(output), "DENIED   resource="+monitorResourceNamespace+"/denied-pod")
		assert.Contains(t, string(output), "MUTATED  resource="+monitorResourceNamespace+"/mutated-pod")
		assert.Contains(t, string(output), "OPERATOR resource="+monitorResourceNamespace+"/package")
		assert.Contains(t, string(output), "ADDED path=/metadata/labels/monitored value=\"true\"")
		assert.NotContains(t, string(output), "filtered-pod")
	})

	t.Run("filters by stream kind", func(t *testing.T) {
		tests := []struct {
			name       string
			stream     string
			contains   []string
			notContain []string
		}{
			{
				name:       "allowed",
				stream:     "allowed",
				contains:   []string{"ALLOWED  resource=" + monitorResourceNamespace + "/allowed-pod"},
				notContain: []string{"DENIED", "MUTATED", "OPERATOR"},
			},
			{
				name:       "operator",
				stream:     "operator",
				contains:   []string{"OPERATOR resource=" + monitorResourceNamespace + "/package"},
				notContain: []string{"ALLOWED", "DENIED", "MUTATED"},
			},
			{
				name:   "failed",
				stream: "failed",
				contains: []string{
					"DENIED   resource=" + monitorResourceNamespace + "/denied-pod",
					"OPERATOR resource=" + monitorResourceNamespace + "/failed-package",
				},
				notContain: []string{"ALLOWED", "MUTATED", monitorResourceNamespace + "/package\n"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				output := testutil.RequireUDSCommand(t, testEnv.udsPath,
					"core", "operator", "monitor", tt.stream,
					"--namespace", monitorResourceNamespace,
					"--no-color",
				)
				for _, expected := range tt.contains {
					assert.Contains(t, string(output), expected)
				}
				for _, unexpected := range tt.notContain {
					assert.NotContains(t, string(output), unexpected)
				}
			})
		}
	})

	t.Run("emits timestamped JSON", func(t *testing.T) {
		output := testutil.RequireUDSCommand(t, testEnv.udsPath,
			"core", "operator", "monitor", "allowed",
			"--namespace", monitorResourceNamespace,
			"--json",
			"--timestamps",
		)

		var event map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(string(output))), &event))
		assert.Equal(t, monitorResourceNamespace, event["namespace"])
		assert.Equal(t, "/allowed-pod", event["name"])
		assert.NotEmpty(t, event["ts"])
	})
}

func deployMonitorPackage(t *testing.T) {
	t.Helper()

	testutil.RequireUDSCommand(t, testEnv.udsPath,
		"zarf", "package", "deploy", testEnv.monitorPackagePath, "--confirm",
	)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), namespaceCleanupTimeout)
		defer cancel()
		if err := testutil.RunCommand(ctx, os.Environ(), testEnv.udsPath,
			"zarf", "package", "remove", testEnv.monitorPackagePath, "--confirm",
		); err != nil {
			t.Errorf("remove operator monitor test package: %v", err)
		}
	})
}

func registerMonitorNamespaceCleanup(t *testing.T, k8s *testutil.K8sClient) {
	t.Helper()

	_, err := k8s.CoreV1().Namespaces().Get(t.Context(), peprSystemNamespace, metav1.GetOptions{})
	if err == nil {
		return
	}
	require.True(t, apierrors.IsNotFound(err), "inspect namespace %q", peprSystemNamespace)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), namespaceCleanupTimeout)
		defer cancel()
		err := k8s.CoreV1().Namespaces().Delete(ctx, peprSystemNamespace, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			t.Errorf("delete namespace %q: %v", peprSystemNamespace, err)
		}
	})
}

func waitForMonitorPod(t *testing.T, k8s *testutil.K8sClient, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), monitorPodTimeout)
	defer cancel()
	err := wait.PollUntilContextCancel(ctx, 2*time.Second, true, func(ctx context.Context) (bool, error) {
		pod, err := k8s.CoreV1().Pods(peprSystemNamespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady {
				return condition.Status == corev1.ConditionTrue, nil
			}
		}
		return false, nil
	})
	if err != nil {
		pod, getErr := k8s.CoreV1().Pods(peprSystemNamespace).Get(t.Context(), name, metav1.GetOptions{})
		require.NoError(t, getErr, "inspect monitor pod %q after readiness failure", name)
		t.Fatalf("wait for monitor pod %q: %v\npod status: %+v", name, err, pod.Status)
	}
}
