// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cli

package zarf_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

// TestRunnerWaitRouting exercises Maru's subprocess callback through both command trees.
func TestRunnerWaitRouting(t *testing.T) {
	uds, err := testutil.ResolveUDSCLIPath()
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	tasksPath := filepath.Join(t.TempDir(), "tasks.yaml")
	var tasks strings.Builder
	tasks.WriteString("tasks:\n  - name: wait\n    actions:\n")
	for _, protocol := range []string{"tcp", "http"} {
		fmt.Fprintf(&tasks, "      - maxTotalSeconds: 10\n        wait:\n          network:\n            protocol: %s\n            address: %s\n", protocol, strings.TrimPrefix(server.URL, "http://"))
	}
	require.NoError(t, os.WriteFile(tasksPath, []byte(tasks.String()), 0o600))
	for _, next := range []bool{false, true} {
		t.Run(fmt.Sprintf("NextMode=%t", next), func(t *testing.T) {
			command := exec.CommandContext(t.Context(), uds, fmt.Sprintf("--features=NextMode=%t", next), "run", "wait", "--file", tasksPath)
			output, err := command.CombinedOutput()
			require.NoError(t, err, string(output))
			require.Equal(t, 2, strings.Count(string(output), "succeeded"), string(output))
		})
	}
}

// TestNextEntrypoint covers behavior that depends on process startup, including
// Zarf's init-time argument inspection. Command contracts otherwise use Cobra directly.
func TestNextEntrypoint(t *testing.T) {
	uds, err := testutil.ResolveUDSCLIPath()
	require.NoError(t, err)
	t.Setenv("CLI_FEATURES", "NextMode=true")
	for _, args := range [][]string{
		{"zarf", "tools", "kubectl", "version", "--client"},
		{"z", "tools", "kubectl", "version", "--client"},
		{"tools", "z", "tools", "kubectl", "version", "--client"},
		{"--features=NextMode", "zarf", "--features=values=true", "tools", "kubectl", "version", "--client"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			output, err := exec.CommandContext(t.Context(), uds, args...).CombinedOutput()
			require.NoError(t, err, string(output))
			require.Contains(t, string(output), "Client Version")
		})
	}
	t.Run("command failure exits once", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		command := exec.CommandContext(t.Context(), uds, "bundle", "create", "--unsigned", "--keyless")
		command.Stdout = &stdout
		command.Stderr = &stderr
		err := command.Run()
		var exitError *exec.ExitError
		require.ErrorAs(t, err, &exitError)
		require.Equal(t, 1, exitError.ExitCode())
		require.Empty(t, stdout.String())
		require.Contains(t, stderr.String(), "--unsigned cannot be combined")
		require.NotContains(t, stderr.String(), "Usage:")
		require.Equal(t, 1, strings.Count(stderr.String(), "--unsigned cannot be combined"), "report the error once")
	})
	t.Run("Zarf failure exits", func(t *testing.T) {
		output, err := exec.CommandContext(t.Context(), uds, "zarf", "package", "inspect", "/does-not-exist.tar.zst").CombinedOutput()
		var exitError *exec.ExitError
		require.ErrorAs(t, err, &exitError)
		require.Equal(t, 1, exitError.ExitCode())
		require.Contains(t, string(output), "does-not-exist")
	})
}
