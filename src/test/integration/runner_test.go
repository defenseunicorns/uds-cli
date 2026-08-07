// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package integration

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestTaskRunner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		want    []string
		notWant []string
		wantErr bool
	}{
		{name: "run action", args: []string{"action"}, want: []string{"specific test string"}},
		{name: "CI disables progress", args: []string{"action"}, env: map[string]string{"CI": "true"}, notWant: []string{"Waiting"}},
		{name: "set variables", args: []string{"cmd-set-variable"}, want: []string{"I'm set from setVariables - unique-value", "I'm set from a runner var - replaced"}},
		{name: "default task", want: []string{"This is the default task"}},
		{name: "missing default task", args: []string{"--file", testutil.TestDataPath("tasks/tasks-no-default.yaml")}, want: []string{"task name default not found"}, wantErr: true},
		{name: "referenced task", args: []string{"reference"}, want: []string{"other-task"}},
		{name: "recursive task", args: []string{"recursive"}, want: []string{"task looping exceeded max configured task stack"}, wantErr: true},
		{name: "set variables from flags", args: []string{"cmd-set-variable", "--set", "REPLACE_ME=replacedWith--setvar", "--set", "UNICORNS=defense"}, want: []string{"I'm set from a runner var - replacedWith--setvar", "I'm set from a new --set var - defense"}},
		{name: "rerun tasks", args: []string{"rerun-tasks"}},
		{name: "rerun child tasks", args: []string{"rerun-tasks-child"}},
		{name: "rerun recursive tasks", args: []string{"rerun-tasks-recursive"}, want: []string{"task looping exceeded max configured task stack"}, wantErr: true},
		{name: "included paths", args: []string{"foobar"}, want: []string{"echo foo", "echo bar"}},
		{name: "variables in included tasks", args: []string{"more-foo", "--set", "FOO_VAR=success"}, want: []string{"success", "foo", "bar"}, notWant: []string{"default"}},
		{name: "list tasks", args: []string{"--list"}, want: []string{"echo-env-var", "Test that env vars take precedence", "remote-import", "action"}},
		{name: "failed wait action", args: []string{"wait-fail"}, want: []string{"Failed to run action"}, wantErr: true},
		{name: "environment file defaults architecture", args: []string{"env-from-file"}, env: map[string]string{"UDS_ARCHITECTURE": ""}, want: []string{runtime.GOARCH, "not-a-secret", "3000", "$env/**/*var with#special%chars!", "env var from calling task - not-a-secret", "overwritten env var - 8080"}},
		{name: "file variable and directory", args: []string{"file-and-dir"}, want: []string{"SECRET_KEY=not-a-secret"}},
		{name: "environment variable overrides default", args: []string{"echo-env-var"}, env: map[string]string{"UDS_TO_BE_OVERWRITTEN": "env-var"}, want: []string{"env-var"}, notWant: []string{"default"}},
		{name: "architecture environment", args: []string{"echo-architecture"}, env: map[string]string{"UDS_ARCHITECTURE": "amd64"}, want: []string{"amd64"}, notWant: []string{"arm64"}},
		{name: "alternate architecture environment", args: []string{"echo-architecture"}, env: map[string]string{"UDS_ARCHITECTURE": "arm64"}, want: []string{"arm64"}, notWant: []string{"amd64"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args := append([]string{"run"}, tt.args...)
			if !containsFileFlag(args) {
				args = append(args, "--file", testutil.TestDataPath("tasks/tasks.yaml"))
			}

			result := runTask(t, tt.env, args...)
			output := result.Stdout + result.Stderr
			if tt.wantErr {
				require.Error(t, result.Err, output)
			} else {
				require.NoError(t, result.Err, result.Stderr)
			}
			for _, want := range tt.want {
				require.Contains(t, output, want)
			}
			for _, notWant := range tt.notWant {
				require.NotContains(t, output, notWant)
			}
		})
	}
}

func TestTaskRunnerSuccessfulWait(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })
	go acceptOneConnection(listener)

	taskDir := testutil.CopyFixture(t, "tasks")
	taskFile := filepath.Join(taskDir, "tasks.yaml")
	contents, err := os.ReadFile(taskFile)
	require.NoError(t, err)
	updated := strings.Replace(string(contents), "githubstatus.com:443", listener.Addr().String(), 1)
	require.NotEqual(t, string(contents), updated)
	require.NoError(t, os.WriteFile(taskFile, []byte(updated), 0o600))

	result := runTask(t, nil, "run", "wait-success", "--file", taskFile)
	require.NoError(t, result.Err, result.Stderr)
	require.Contains(t, result.Stdout+result.Stderr, "succeeded")
}

func acceptOneConnection(listener net.Listener) {
	connection, err := listener.Accept()
	if err == nil {
		_ = connection.Close()
	}
}

func TestTaskRunnerIncludedTaskLoop(t *testing.T) {
	t.Parallel()

	taskDir := testutil.CopyFixture(t, "tasks")
	taskFile := filepath.Join(taskDir, "tasks.yaml")
	loopingTask := []byte("includes:\n  - infinite: ./loop-task.yaml\n\ntasks:\n  - name: include-loop\n    actions:\n      - task: infinite:loop\n")
	require.NoError(t, os.WriteFile(taskFile, loopingTask, 0o600))

	result := runTask(t, nil, "run", "include-loop", "--file", taskFile)
	output := result.Stdout + result.Stderr
	require.Error(t, result.Err, output)
	require.Contains(t, output, "task looping exceeded max configured task stack")
}

func runTask(t *testing.T, env map[string]string, args ...string) testutil.CommandResult {
	t.Helper()

	workspace := t.TempDir()
	command := []string{
		"--tmpdir", workspace,
	}
	command = append(command, args...)
	opts := isolatedOptions(t, env)
	opts.Dir = repositoryRoot()
	return testutil.RunCLI(t, opts, command...)
}

func repositoryRoot() string {
	return filepath.Dir(filepath.Dir(filepath.Dir(testutil.TestDataPath("tasks"))))
}

func containsFileFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--file" {
			return true
		}
	}
	return false
}
