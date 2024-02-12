// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunnerInputs(t *testing.T) {
	t.Run("test that default values for inputs work when not required", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "has-default-empty", "--file", "src/test/tasks/inputs/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "default")
		require.NotContains(t, stdErr, "{{")
	})

	t.Run("test that default values for inputs work when required", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "has-default-and-required-empty", "--file", "src/test/tasks/inputs/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "default")
		require.NotContains(t, stdErr, "{{")

	})

	t.Run("test that default values for inputs work when required and have values supplied", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "has-default-and-required-supplied", "--file", "src/test/tasks/inputs/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "supplied-value")
		require.NotContains(t, stdErr, "{{")
	})

	t.Run("test that inputs that aren't required with no default don't error", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "no-default-empty", "--file", "src/test/tasks/inputs/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.NotContains(t, stdErr, "has-no-default")
		require.NotContains(t, stdErr, "{{")
	})

	t.Run("test that inputs with no defaults that aren't required don't error when supplied with a value", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "no-default-supplied", "--file", "src/test/tasks/inputs/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "success + supplied-value")
		require.NotContains(t, stdErr, "{{")
	})

	t.Run("test that tasks that require inputs with no defaults error when called without values", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "no-default-and-required-empty", "--file", "src/test/tasks/inputs/tasks.yaml")
		require.Error(t, err, stdOut, stdErr)
		require.NotContains(t, stdErr, "{{")
	})

	t.Run("test that tasks that require inputs with no defaults run when supplied with a value", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "no-default-and-required-supplied", "--file", "src/test/tasks/inputs/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "supplied-value")
		require.NotContains(t, stdErr, "{{")
	})

	t.Run("test that when a task is called with extra inputs it warns", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "no-default-and-required-supplied-extra", "--file", "src/test/tasks/inputs/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "supplied-value")
		require.Contains(t, stdErr, "WARNING")
		require.Contains(t, stdErr, "does not have an input named extra")
		require.NotContains(t, stdErr, "{{")
	})

	t.Run("test that displays a deprecated message", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "deprecated-task", "--file", "src/test/tasks/inputs/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "WARNING")
		require.Contains(t, stdErr, "This input has been marked deprecated: This is a deprecated message")
	})

	t.Run("test that variables can be used as inputs", func(t *testing.T) {
		t.Parallel()

		stdOut, stdErr, err := e2e.UDS("run", "variable-as-input", "--file", "src/test/tasks/inputs/tasks.yaml", "--set", "foo=im a variable")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "im a variable")
	})

	t.Run("test that env vars can be used as inputs and take precedence over default vals", func(t *testing.T) {
		os.Setenv("UDS_FOO", "im an env var")
		stdOut, stdErr, err := e2e.UDS("run", "variable-as-input", "--file", "src/test/tasks/inputs/tasks.yaml")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "im an env var")
	})

	t.Run("test that a --set var has the greatest precedence for inputs", func(t *testing.T) {
		os.Setenv("UDS_FOO", "im an env var")
		stdOut, stdErr, err := e2e.UDS("run", "variable-as-input", "--file", "src/test/tasks/inputs/tasks.yaml", "--set", "foo=most specific")
		require.NoError(t, err, stdOut, stdErr)
		require.Contains(t, stdErr, "most specific")
	})
}
