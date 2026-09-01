// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package core

import (
	"context"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/internal/operator"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperatorMonitorCommand_Run(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	var received operator.MonitorOptions
	monitor := func(_ context.Context, _ iostreams.IOStreams, opts operator.MonitorOptions) error {
		received = opts
		return nil
	}
	cmd := newOperatorMonitorCommand(streams, monitor)
	cmd.PersistentFlags().String("log-level", "info", "")
	cmd.SetArgs([]string{
		"denied",
		"--namespace", "tenant",
		"--follow",
		"--timestamps",
		"--since", "5m",
		"--json",
		"--no-color",
		"--log-level", "error",
	})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, operator.StreamDenied, received.Stream)
	assert.Equal(t, "tenant", received.Namespace)
	assert.True(t, received.Follow)
	assert.True(t, received.Timestamps)
	assert.Equal(t, 5*time.Minute, received.Since)
	assert.True(t, received.JSON)
	assert.True(t, received.NoColor)
	assert.Equal(t, "error", received.LogLevel)
}

func TestOperatorMonitorOptions_Validate(t *testing.T) {
	tests := []struct {
		name    string
		options OperatorMonitorOptions
		wantErr string
	}{
		{name: "valid", options: OperatorMonitorOptions{Stream: operator.StreamPolicies}},
		{name: "invalid stream", options: OperatorMonitorOptions{Stream: "unknown"}, wantErr: "invalid stream kind: unknown"},
		{name: "negative since", options: OperatorMonitorOptions{Since: -time.Second}, wantErr: "since must not be negative"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.options.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.wantErr)
		})
	}
}
