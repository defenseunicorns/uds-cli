// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateConfig_Nil(t *testing.T) {
	err := ValidateConfig(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config is required")
}

func TestValidateConfig_NilGlobal(t *testing.T) {
	err := ValidateConfig(&UDSBundleConfig{
		Options: &ConfigOptions{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config.Global is required")
}

func TestValidateConfig_NilOptions(t *testing.T) {
	err := ValidateConfig(&UDSBundleConfig{
		Global: &GlobalOptions{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config.Options is required")
}

func TestValidateConfig_Valid(t *testing.T) {
	err := ValidateConfig(&UDSBundleConfig{
		Global:  &GlobalOptions{},
		Options: &ConfigOptions{},
	})
	require.NoError(t, err)
}
