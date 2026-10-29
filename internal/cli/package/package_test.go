// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package packagecli

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageModRejectsSourceIdentityOverrides(t *testing.T) {
	// Reassembly preserves recovered architecture and flavor and omits source-time
	// variable overrides.
	tests := []struct {
		name string
		args []string
	}{
		{name: "reassemble architecture", args: []string{"reassemble", "source", "--architecture=arm64"}},
		{name: "reassemble flavor", args: []string{"reassemble", "source", "--flavor=offline"}},
		{name: "reassemble variables", args: []string{"reassemble", "source", "--set=VALUE=changed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			cmd := NewModCommand(streams)
			cmd.SetArgs(tt.args)
			require.ErrorContains(t, cmd.Execute(), "unknown flag")
		})
	}
}

func TestDisassembleArchitectureSelection(t *testing.T) {
	for _, flag := range []string{"--architecture", "-a"} {
		t.Run(flag, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			parent := NewModCommand(streams)
			cmd, _, err := parent.Find([]string{"disassemble"})
			require.NoError(t, err)
			require.NoError(t, cmd.ParseFlags([]string{flag, "arm64"}))
			opts := NewDisassembleOptions(streams)
			require.NoError(t, opts.Complete(cmd, []string{"source", "output"}))
			assert.Equal(t, "arm64", opts.Architecture)
		})
	}
}

func TestReassembleAcceptsPackagingControls(t *testing.T) {
	// These controls affect package creation without changing the recovered
	// package identity or reintroducing source-time template inputs.
	streams, _, out, _ := iostreams.NewTestIOStreams()
	cmd := NewModCommand(streams)
	cmd.SetOut(out)
	cmd.SetArgs([]string{"reassemble", "--help"})
	require.NoError(t, cmd.Execute())
	help := out.String()
	assert.Contains(t, help, "-o, --output")
	assert.Contains(t, help, "-m, --max-package-size")
	assert.Contains(t, help, "--cache")
	assert.Contains(t, help, "--signing-key")
	assert.Contains(t, help, "--signing-key-pass")
	assert.Contains(t, help, "--with-build-machine-info")
}
