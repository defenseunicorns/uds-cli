// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package core

import (
	"context"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/internal/operator"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
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
	require.Equal(t, operator.StreamDenied, received.Stream)
	require.Equal(t, "tenant", received.Namespace)
	require.True(t, received.Follow)
	require.True(t, received.Timestamps)
	require.Equal(t, 5*time.Minute, received.Since)
	require.True(t, received.JSON)
	require.True(t, received.NoColor)
	require.Equal(t, "error", received.LogLevel)
}

func TestOperatorMonitorOptions_Validate(t *testing.T) {
	tests := []struct {
		name       string
		options    OperatorMonitorOptions
		wantErr    error
		wantErrMsg string
	}{
		{name: "valid", options: OperatorMonitorOptions{Stream: operator.StreamPolicies}},
		{
			name:       "invalid stream",
			options:    OperatorMonitorOptions{Stream: "unknown"},
			wantErr:    ErrInvalidStreamKind,
			wantErrMsg: "invalid stream kind: unknown",
		},
		{
			name:       "negative since",
			options:    OperatorMonitorOptions{Since: -time.Second},
			wantErr:    ErrInvalidSince,
			wantErrMsg: "since must not be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.options.Validate()
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, tt.wantErrMsg, err.Error())
		})
	}
}
