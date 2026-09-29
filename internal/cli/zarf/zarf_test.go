// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestZarfCommandReturnsExecutionErrors(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })

	cmd := NewInternalZarfCommand()
	cmd.SetArgs([]string{"--architecture"})
	require.Error(t, cmd.Execute())
}
