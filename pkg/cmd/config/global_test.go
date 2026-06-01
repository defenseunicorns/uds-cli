// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveGlobalOptions_Defaults(t *testing.T) {
	global, err := ResolveGlobalOptions(false, "info", "info")
	require.NoError(t, err)

	assert.Equal(t, "info", global.LogLevel)
	assert.False(t, global.Prompt)
}

func TestResolveGlobalOptions_PromptFlag(t *testing.T) {
	global, err := ResolveGlobalOptions(true, "info", "info")
	require.NoError(t, err)

	assert.True(t, global.Prompt)
}

func TestResolveGlobalOptions_EffectiveLogLevel(t *testing.T) {
	// Simulate HCL setting log level to "debug" while --log-level flag stays at default "info"
	global, err := ResolveGlobalOptions(false, "info", "debug")
	require.NoError(t, err)

	assert.Equal(t, "debug", global.LogLevel)
}

func TestResolveGlobalOptions_CLILogLevelMatchesEffective(t *testing.T) {
	// When CLI flag matches effective level, no logger re-init is needed
	global, err := ResolveGlobalOptions(false, "warn", "warn")
	require.NoError(t, err)

	assert.Equal(t, "warn", global.LogLevel)
}

func TestResolveGlobalOptions_InvalidLogLevel(t *testing.T) {
	_, err := ResolveGlobalOptions(false, "info", "invalid-level")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log level")
}

func TestResolveGlobalOptions_NoPrompt(t *testing.T) {
	global, err := ResolveGlobalOptions(false, "info", "info")
	require.NoError(t, err)

	assert.False(t, global.Prompt, "prompt=false should be reflected in GlobalOptions")
}
