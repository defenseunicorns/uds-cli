// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package core

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperatorMonitorCommand_Run(t *testing.T) {
	streams, _, out, _ := iostreams.NewTestIOStreams()
	cmd := NewCoreCommand(streams)
	cmd.SetArgs([]string{"operator", "monitor", "--no-color"})

	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "MUTATED  resource=istio-system/istiod")
	assert.Contains(t, out.String(), "ALLOWED  resource=istio-system/istiod repeated=1")
	assert.Contains(t, out.String(), "DENIED   resource=default/bad-pod")
	assert.Contains(t, out.String(), "ADDED path=/spec/securityContext/runAsNonRoot value=true")
}
