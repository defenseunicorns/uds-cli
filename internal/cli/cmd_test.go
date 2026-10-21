// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package cli

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/mode"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
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
			root := NewRootCommand(streams, mode.FeatureSet{})
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

func TestRootCommand_EmptyArchitectureFlagFails(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	root := NewRootCommand(streams, mode.FeatureSet{})
	root.SetArgs([]string{"bundle", "create", "--architecture=", "--unsigned"})

	err := root.Execute()
	require.ErrorContains(t, err, "--architecture must not be empty")
}

func TestRootCommand_PackageModFeatureGate(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	disabled := NewRootCommand(streams, mode.FeatureSet{mode.FeaturePackageMod: false})
	assert.Nil(t, childCommand(disabled, "package"))

	enabled := NewRootCommand(streams, mode.FeatureSet{mode.FeaturePackageMod: true})
	packageCmd := childCommand(enabled, "package")
	require.NotNil(t, packageCmd)
	modCmd := childCommand(packageCmd, "mod")
	require.NotNil(t, modCmd)
	assert.NotNil(t, childCommand(modCmd, "disassemble"))
	assert.NotNil(t, childCommand(modCmd, "reassemble"))
}

func childCommand(parent interface{ Commands() []*cobra.Command }, name string) *cobra.Command {
	for _, cmd := range parent.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}
