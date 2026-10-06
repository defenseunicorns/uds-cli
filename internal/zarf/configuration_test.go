// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zarfconfig "github.com/zarf-dev/zarf/src/config"
)

func TestWithTempDirConfiguresZarfOnce(t *testing.T) {
	resetTempDirConfig(t)
	zarfconfig.CommonOptions.TempDirectory = "before"
	wantErr := errors.New("operation failed")

	result, err := WithTempDir("during", func() (string, error) {
		assert.Equal(t, "during", zarfconfig.CommonOptions.TempDirectory)
		return "result", wantErr
	})

	assert.Equal(t, "result", result)
	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, "during", zarfconfig.CommonOptions.TempDirectory)
}

func TestWithTempDirRejectsAnotherDirectory(t *testing.T) {
	resetTempDirConfig(t)
	_, err := WithTempDir("first", func() (struct{}, error) {
		return struct{}{}, nil
	})
	require.NoError(t, err)

	called := false
	_, err = WithTempDir("second", func() (struct{}, error) {
		called = true
		return struct{}{}, nil
	})

	require.ErrorContains(t, err, `zarf temporary directory is already configured as "first", cannot change it to "second"`)
	assert.False(t, called)
	assert.Equal(t, "first", zarfconfig.CommonOptions.TempDirectory)
}

func resetTempDirConfig(t *testing.T) {
	t.Helper()

	zarfTempDir.Lock()
	originalValue := zarfTempDir.value
	originalSet := zarfTempDir.set
	originalZarfValue := zarfconfig.CommonOptions.TempDirectory
	zarfTempDir.value = ""
	zarfTempDir.set = false
	zarfTempDir.Unlock()

	t.Cleanup(func() {
		zarfTempDir.Lock()
		zarfTempDir.value = originalValue
		zarfTempDir.set = originalSet
		zarfconfig.CommonOptions.TempDirectory = originalZarfValue
		zarfTempDir.Unlock()
	})
}
