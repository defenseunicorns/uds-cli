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
		wantMod bool
	}{
		{name: "disabled by default", args: []string{"package", "mod"}},
		{name: "enabled by flag", args: []string{"--features=PackageMod", "package", "mod"}, wantMod: true},
		{name: "enabled by environment", args: []string{"package", "mod"}, env: "PackageMod", wantMod: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			lookupEnv := func(name string) (string, bool) {
				require.Equal(t, mode.FeaturesEnv, name)
				return tt.env, tt.env != ""
			}
			root, _, err := newRootCommand(tt.args, lookupEnv, streams)
			require.NoError(t, err)
			found := false
			for _, cmd := range root.Commands() {
				if cmd.Name() == "package" {
					found = true
				}
			}
			require.Equal(t, tt.wantMod, found)
		})
	}
}
