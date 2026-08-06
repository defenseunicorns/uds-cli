// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
	"github.com/stretchr/testify/require"
)

func TestLoadVariablesExcludesCLIMode(t *testing.T) {
	for _, value := range []string{"legacy", "next"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(config.CLIModeEnvVar, value)

			bundle := newTestBundle(nil, nil, nil, "", "")
			variables, overrides := bundle.loadVariables(types.Package{}, nil)

			require.NotContains(t, variables, "CLI_MODE")
			require.NotContains(t, overrides, "CLI_MODE")
		})
	}
}
