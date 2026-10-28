// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package main

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/mode"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func TestNewRootCommandPackageModFeatureGate(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     string
		enabled bool
	}{
		{name: "disabled by default"},
		{name: "enabled by flag", args: []string{"--features=PackageMod"}, enabled: true},
		{name: "enabled by environment", env: "PackageMod", enabled: true},
		{name: "disabled by flag overrides environment", args: []string{"--features=PackageMod=false"}, env: "PackageMod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookupEnv := func(name string) (string, bool) {
				require.Equal(t, mode.FeaturesEnv, name)
				return tt.env, tt.env != ""
			}
			for _, command := range []struct {
				name    string
				args    []string
				wantErr string
			}{
				{name: "disassemble", args: []string{"", ""}, wantErr: "source is required"},
				{name: "reassemble", args: []string{""}, wantErr: "source directory is required"},
			} {
				t.Run(command.name, func(t *testing.T) {
					streams, _, _, _ := iostreams.NewTestIOStreams()
					args := append([]string(nil), tt.args...)
					args = append(args, "package", "mod", command.name)
					args = append(args, command.args...)
					wantErr := "package modification requires the PackageMod feature flag"
					if tt.enabled {
						wantErr = command.wantErr
					}
					require.ErrorContains(t, run(args, lookupEnv, streams), wantErr)
				})
			}
		})
	}
}
