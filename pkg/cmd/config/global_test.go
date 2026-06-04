// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveGlobalOptions_Defaults(t *testing.T) {
	global, err := ResolveGlobalOptions(false, "info")
	require.NoError(t, err)

	assert.Equal(t, "info", global.LogLevel)
	assert.False(t, global.Prompt)
}

func TestResolveGlobalOptions_PromptFlag(t *testing.T) {
	global, err := ResolveGlobalOptions(true, "info")
	require.NoError(t, err)

	assert.True(t, global.Prompt)
}

func TestResolveGlobalOptions_LogLevelCarriedThrough(t *testing.T) {
	global, err := ResolveGlobalOptions(false, "debug")
	require.NoError(t, err)

	assert.Equal(t, "debug", global.LogLevel)
}

func TestResolveGlobalOptions_InvalidLogLevel(t *testing.T) {
	_, err := ResolveGlobalOptions(false, "invalid-level")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log level")
}

func TestResolveGlobalOptions_NoPrompt(t *testing.T) {
	global, err := ResolveGlobalOptions(false, "info")
	require.NoError(t, err)

	assert.False(t, global.Prompt, "prompt=false should be reflected in GlobalOptions")
}
