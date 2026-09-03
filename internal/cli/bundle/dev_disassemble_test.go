// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDevCommandContainsDisassemble(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	cmd := NewDevCommand(streams)
	found, _, err := cmd.Find([]string{"disassemble"})
	require.NoError(t, err)
	assert.Equal(t, "disassemble", found.Name())
	assert.Equal(t, "disassemble <source> <output-dir>", found.Use)
	assert.Equal(t, "[beta] Convert a Zarf package into rebuildable offline source", found.Short)
	assert.Equal(t, "[beta] Extract a complete Zarf package and rewrite its packaged resources into a local source directory that can be rebuilt offline. Zarf packages are supported today.", found.Long)
}
