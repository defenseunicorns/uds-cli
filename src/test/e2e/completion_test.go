// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package test provides e2e tests for UDS.
package test

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/stretchr/testify/require"
)

func TestCompletion(t *testing.T) {

	t.Run("Test Completion", func(t *testing.T) {
		output, _ := runCmd(t, "completion")
		require.Contains(t, output, lang.CompletionCmdLong)
	})

	t.Run("Test Bash Completion", func(t *testing.T) {
		output, _ := runCmd(t, "completion bash")
		require.Contains(t, output, "bash completion V2 for uds")
	})

	t.Run("Test ZSH Completion", func(t *testing.T) {
		output, _ := runCmd(t, "completion zsh")
		require.Contains(t, output, "zsh completion for uds")
	})

	t.Run("Test Fish Completion", func(t *testing.T) {
		output, _ := runCmd(t, "completion fish")
		require.Contains(t, output, "fish completion for uds")
	})
}
