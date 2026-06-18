// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package cmd

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func TestRootCommand_LogLevelFlag(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		wantErr bool
	}{
		{name: "valid debug level", level: "debug"},
		{name: "valid info level", level: "info"},
		{name: "valid warn level", level: "warn"},
		{name: "valid error level", level: "error"},
		{name: "invalid level errors", level: "garbage", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			root := NewRootCommand(streams)
			root.SetArgs([]string{"--log-level", tt.level, "version"})

			err := root.Execute()
			if tt.wantErr {
				require.ErrorContains(t, err, "unknown log level")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
