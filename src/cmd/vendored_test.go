// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors
package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestAddScanCmdFlags(t *testing.T) {
	cmd := &cobra.Command{}
	addScanCmdFlags(cmd)

	tests := []struct {
		name         string
		flagName     string
		expectedType string
		expectedDef  string
	}{
		{"docker-username", "docker-username", "string", ""},
		{"docker-password", "docker-password", "string", ""},
		{"org", "org", "string", "defenseunicorns"},
		{"package-name", "package-name", "string", ""},
		{"tag", "tag", "string", ""},
		{"output-file", "output-file", "string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := cmd.PersistentFlags().Lookup(tt.flagName)
			assert.NotNil(t, flag)
			assert.Equal(t, tt.expectedType, flag.Value.Type())
			assert.Equal(t, tt.expectedDef, flag.DefValue)
		})
	}
}
