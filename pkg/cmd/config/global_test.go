// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package config

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// registerGlobalFlags mirrors the persistent flags from the root command.
func registerGlobalFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("log-level", "l", "info", "log level")
	cmd.Flags().Bool("prompt", false, "enable interactive confirmation prompts")
}

func TestResolveGlobalOptions_Defaults(t *testing.T) {
	cmd := &cobra.Command{}
	registerGlobalFlags(cmd)

	global, err := ResolveGlobalOptions(cmd, "info")
	require.NoError(t, err)

	assert.Equal(t, "info", global.LogLevel)
	assert.False(t, global.Prompt)
}

func TestResolveGlobalOptions_PromptFlag(t *testing.T) {
	cmd := &cobra.Command{}
	registerGlobalFlags(cmd)
	require.NoError(t, cmd.Flags().Set("prompt", "true"))

	global, err := ResolveGlobalOptions(cmd, "info")
	require.NoError(t, err)

	assert.True(t, global.Prompt)
}

func TestResolveGlobalOptions_EffectiveLogLevel(t *testing.T) {
	cmd := &cobra.Command{}
	registerGlobalFlags(cmd)

	// Simulate HCL setting log level to "debug" while --log-level flag stays at default "info"
	global, err := ResolveGlobalOptions(cmd, "debug")
	require.NoError(t, err)

	assert.Equal(t, "debug", global.LogLevel)
}

func TestResolveGlobalOptions_CLILogLevelMatchesEffective(t *testing.T) {
	cmd := &cobra.Command{}
	registerGlobalFlags(cmd)
	require.NoError(t, cmd.Flags().Set("log-level", "warn"))

	// When CLI flag matches effective level, no logger re-init is needed
	global, err := ResolveGlobalOptions(cmd, "warn")
	require.NoError(t, err)

	assert.Equal(t, "warn", global.LogLevel)
}

func TestResolveGlobalOptions_InvalidLogLevel(t *testing.T) {
	cmd := &cobra.Command{}
	registerGlobalFlags(cmd)

	_, err := ResolveGlobalOptions(cmd, "invalid-level")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log level")
}

func TestResolveGlobalOptions_NoPromptFlag(t *testing.T) {
	// When --prompt flag is not registered, GetBool returns false (zero value)
	cmd := &cobra.Command{}
	cmd.Flags().StringP("log-level", "l", "info", "log level")

	global, err := ResolveGlobalOptions(cmd, "info")
	require.NoError(t, err)

	assert.False(t, global.Prompt, "missing prompt flag should default to false")
}
