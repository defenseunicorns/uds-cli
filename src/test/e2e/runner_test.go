// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskRunner(t *testing.T) {
	t.Log("E2E: Task Runner")

	t.Run("run copy", func(t *testing.T) {
		t.Parallel()

		baseFilePath := "base"
		copiedFilePath := "copy"

		e2e.CleanFiles(baseFilePath, copiedFilePath)
		t.Cleanup(func() {
			e2e.CleanFiles(baseFilePath, copiedFilePath)
		})

		err := os.WriteFile(baseFilePath, []byte{}, 0600)
		require.NoError(t, err)

		stdOut, stdErr, err := e2e.UDS("run", "copy", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)

		require.FileExists(t, copiedFilePath)
	})

	t.Run("run copy-exec", func(t *testing.T) {
		t.Parallel()

		baseFilePath := "exectest"
		copiedFilePath := "exec"

		e2e.CleanFiles(baseFilePath, copiedFilePath)
		t.Cleanup(func() {
			e2e.CleanFiles(baseFilePath, copiedFilePath)
		})

		err := os.WriteFile(baseFilePath, []byte{}, 0600)
		require.NoError(t, err)

		stdOut, stdErr, err := e2e.UDS("run", "copy-exec", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)

		require.FileExists(t, copiedFilePath)
		execFileInfo, err := os.Stat(copiedFilePath)
		require.NoError(t, err)
		require.True(t, execFileInfo.Mode()&0111 != 0)
	})

	t.Run("run copy-verify", func(t *testing.T) {
		t.Parallel()

		baseFilePath := "data"
		copiedFilePath := "verify"

		e2e.CleanFiles(baseFilePath, copiedFilePath)
		t.Cleanup(func() {
			e2e.CleanFiles(baseFilePath, copiedFilePath)
		})

		err := os.WriteFile(baseFilePath, []byte("test"), 0600)
		require.NoError(t, err)

		stdOut, stdErr, err := e2e.UDS("run", "copy-verify", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)

		require.FileExists(t, copiedFilePath)
	})

	t.Run("run copy-symlink", func(t *testing.T) {
		t.Parallel()

		baseFilePath := "symtest"
		copiedFilePath := "symcopy"
		symlinkName := "testlink"

		e2e.CleanFiles(baseFilePath, copiedFilePath, symlinkName)
		t.Cleanup(func() {
			e2e.CleanFiles(baseFilePath, copiedFilePath, symlinkName)
		})

		err := os.WriteFile(baseFilePath, []byte{}, 0600)
		require.NoError(t, err)

		stdOut, stdErr, err := e2e.UDS("run", "copy-symlink", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)

		require.FileExists(t, symlinkName)
	})

	t.Run("run local-import-with-curl", func(t *testing.T) {
		t.Parallel()

		downloadedFile := "checksums.txt"

		e2e.CleanFiles(downloadedFile)
		t.Cleanup(func() {
			e2e.CleanFiles(downloadedFile)
		})
		// get current git revision
		gitRev, err := e2e.GetGitRevision()
		if err != nil {
			return
		}
		setVar := fmt.Sprintf("GIT_REVISION=%s", gitRev)
		stdOut, stdErr, err := e2e.UDS("run", "local-import-with-curl", "--set", setVar, "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)

		require.FileExists(t, downloadedFile)
	})

	t.Run("run template-file", func(t *testing.T) {
		t.Parallel()

		baseFilePath := "raw"
		copiedFilePath := "templated"

		e2e.CleanFiles(baseFilePath, copiedFilePath)
		t.Cleanup(func() {
			e2e.CleanFiles(baseFilePath, copiedFilePath)
		})

		err := os.WriteFile(baseFilePath, []byte("${REPLACE_ME}"), 0600)
		require.NoError(t, err)

		stdOut, stdErr, err := e2e.UDS("run", "template-file", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)

		require.FileExists(t, copiedFilePath)

		templatedContentsBytes, err := os.ReadFile(copiedFilePath)
		require.NoError(t, err)
		require.Equal(t, "replaced\n", string(templatedContentsBytes))
	})

	t.Run("run action", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "action", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "specific test string")
	})

	t.Run("run cmd-set-variable", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "cmd-set-variable", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "I'm set from setVariables - unique-value")
		require.Contains(t, stdErr, "I'm set from a runner var - replaced")
	})
	t.Run("run default task", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "This is the default task")

	})

	t.Run("run default task when undefined", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "--file", "src/test/tasks/tasks-no-default.yaml")
		require.Error(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "task name default not found")
	})
	t.Run("run reference", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "reference", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "other-task")
	})

	t.Run("run recursive", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "recursive", "--file", "src/test/tasks/tasks.yaml")
		require.Error(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "task loop detected")
	})

	t.Run("includes task loop", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "include-loop", "--file", "src/test/tasks/tasks.yaml")
		require.Error(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "task loop detected")
	})

	t.Run("run cmd-set-variable with --set", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "cmd-set-variable", "--set", "REPLACE_ME=replacedWith--setvar", "--set", "UNICORNS=defense", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "I'm set from a runner var - replacedWith--setvar")
		require.Contains(t, stdErr, "I'm set from a new --set var - defense")
	})

	t.Run("run remote-import", func(t *testing.T) {
		t.Parallel()

		// get current git revision
		gitRev, err := e2e.GetGitRevision()
		if err != nil {
			return
		}
		setVar := fmt.Sprintf("GIT_REVISION=%s", gitRev)
		stdOut, stdErr, err := e2e.UDS("run", "remote-import", "--set", setVar, "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "defenseunicorns is a pretty ok company")
	})

	t.Run("run rerun-tasks", func(t *testing.T) {
		t.Parallel()
		stdOut, stdErr, err := e2e.UDS("run", "rerun-tasks", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
	})

	t.Run("run rerun-tasks-child", func(t *testing.T) {
		t.Parallel()
		stdOut, stdErr, err := e2e.UDS("run", "rerun-tasks-child", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
	})

	t.Run("run rerun-tasks-recursive", func(t *testing.T) {
		t.Parallel()
		stdOut, stdErr, err := e2e.UDS("run", "rerun-tasks-recursive", "--file", "src/test/tasks/tasks.yaml")
		require.Error(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "task loop detected")
	})

	t.Run("test includes paths", func(t *testing.T) {
		t.Parallel()
		stdOut, stdErr, err := e2e.UDS("run", "foobar", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "echo foo")
		require.Contains(t, stdErr, "echo bar")
	})

	t.Run("test action with multiple include tasks", func(t *testing.T) {
		t.Parallel()
		gitRev, err := e2e.GetGitRevision()
		if err != nil {
			return
		}
		setVar := fmt.Sprintf("GIT_REVISION=%s", gitRev)

		stdOut, stdErr, err := e2e.UDS("run", "more-foobar", "--set", setVar, "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "echo foo")
		require.Contains(t, stdErr, "echo bar")
		require.Contains(t, stdErr, "defenseunicorns is a pretty ok company")
	})

	t.Run("test action with multiple nested include tasks", func(t *testing.T) {
		t.Parallel()
		gitRev, err := e2e.GetGitRevision()
		if err != nil {
			return
		}
		setVar := fmt.Sprintf("GIT_REVISION=%s", gitRev)

		stdOut, stdErr, err := e2e.UDS("run", "extra-foobar", "--set", setVar, "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "echo foo")
		require.Contains(t, stdErr, "echo bar")
		require.Contains(t, stdErr, "defenseunicorns")
		require.Contains(t, stdErr, "defenseunicorns is a pretty ok company")
	})

	t.Run("test variable passing to included tasks", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "more-foo", "--set", "FOO_VAR=success", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "success")
		require.Contains(t, stdErr, "foo")
		require.Contains(t, stdErr, "bar")
		require.NotContains(t, stdErr, "default")
	})

	t.Run("run list tasks", func(t *testing.T) {
		t.Parallel()
		gitRev, err := e2e.GetGitRevision()
		if err != nil {
			return
		}
		setVar := fmt.Sprintf("GIT_REVISION=%s", gitRev)

		stdOut, stdErr, err := e2e.UDS("run", "--list", setVar, "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "copy")
		require.Contains(t, stdErr, "This is a copy task")
		require.Contains(t, stdErr, "copy-exec")
		require.Contains(t, stdErr, "copy-verify")
	})

	t.Run("test call to zarf tools wait-for", func(t *testing.T) {
		t.Parallel()
		_, stdErr, err := e2e.UDS("run", "wait", "--file", "src/test/tasks/tasks.yaml")
		require.Error(t, err)
		require.Contains(t, stdErr, "Waiting for")
	})

	t.Run("test task to load env vars using the envPath key", func(t *testing.T) {
		t.Parallel()
		stdOut, stdErr, err := e2e.UDS("run", "env-from-file", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, e2e.Arch)
		require.Contains(t, stdErr, "not-a-secret")
		require.Contains(t, stdErr, "3000")
		require.Contains(t, stdErr, "$env/**/*var with#special%chars!")
		require.Contains(t, stdErr, "env var from calling task - not-a-secret")
		require.Contains(t, stdErr, "overwritten env var - 8080")
	})

	t.Run("test that variables of type file and setting dir from a variable are processed correctly", func(t *testing.T) {
		t.Parallel()
		stdOut, stdErr, err := e2e.UDS("run", "file-and-dir", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "SECRET_KEY=not-a-secret")
	})

	t.Run("test that env vars get used for variables that do not have a default set", func(t *testing.T) {
		t.Parallel()
		os.Setenv("UDS_REPLACE_ME", "env-var")
		os.Setenv("UDS_NO_DEFAULT", "no-problem")
		stdOut, stdErr, err := e2e.UDS("run", "echo-env-var", "--file", "src/test/tasks/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "replaced")
		require.NotContains(t, stdErr, "env-var")
		require.Contains(t, stdErr, "no-problem")
	})
}
