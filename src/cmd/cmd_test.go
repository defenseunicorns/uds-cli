package cmd

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
)

func TestSettingArchitecture(t *testing.T) {
	testCases := []struct {
		name        string
		cliArch     string
		bundle      *types.UDSBundle
		expectedVal string
	}{
		{
			name:    "CLIArch takes precedence",
			cliArch: "archFromFlag",
			bundle: &types.UDSBundle{
				Metadata: types.UDSMetadata{
					Architecture: "setFromMetadata",
				},
			},
			expectedVal: "archFromFlag",
		},
		{
			name:    "Metadata.Arch",
			cliArch: "",
			bundle: &types.UDSBundle{
				Metadata: types.UDSMetadata{
					Architecture: "setFromMetadata",
				},
			},
			expectedVal: "setFromMetadata",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.bundle
			config.CLIArch = tc.cliArch
			b.Build.Architecture = config.GetArch(b.Metadata.Architecture)
			require.Equal(t, tc.expectedVal, b.Build.Architecture)
		})
	}
}
