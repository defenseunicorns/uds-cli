// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/stretchr/testify/require"
)

func TestCompletion(t *testing.T) {

	t.Run("Test Completion", func(t *testing.T) {
		cmd := strings.Split("completion", " ")
		output, _, _ := e2e.UDS(cmd...)
		require.Contains(t, output, lang.CompletionCmdLong)
	})

	t.Run("Test Bash Completion", func(t *testing.T) {
		cmd := strings.Split("completion bash", " ")
		output, _, _ := e2e.UDS(cmd...)
		require.Contains(t, output, "bash completion V2 for uds")
	})

	t.Run("Test ZSH Completion", func(t *testing.T) {
		cmd := strings.Split("completion zsh", " ")
		output, _, _ := e2e.UDS(cmd...)
		require.Contains(t, output, "zsh completion for uds")
	})

	t.Run("Test Fish Completion", func(t *testing.T) {
		cmd := strings.Split("completion fish", " ")
		output, _, _ := e2e.UDS(cmd...)
		require.Contains(t, output, "fish completion for uds")
	})
}
