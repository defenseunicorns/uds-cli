// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/stretchr/testify/require"
)

func TestNewRemoveOptionsUsesPackageNamespace(t *testing.T) {
	t.Parallel()

	removeOpts := newRemoveOptions("podinfo-system", nil)

	require.Equal(t, "podinfo-system", removeOpts.NamespaceOverride)
	require.Equal(t, config.HelmTimeout, removeOpts.Timeout)
	require.Nil(t, removeOpts.Cluster)
}
