// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"log/slog"
	"os"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/kube"
)

func TestExecuteCLIRestoresProcessStateAfterError(t *testing.T) {
	before, err := os.Getwd()
	require.NoError(t, err)
	logger, fieldManager := slog.Default(), kube.ManagedFieldsManager
	streams, _, _, _ := iostreams.NewTestIOStreams()

	err = executeCLI(t.Context(), streams, t.TempDir(), "bundle", "create", "--unsigned", "--keyless")
	require.ErrorContains(t, err, "--unsigned cannot be combined")

	after, err := os.Getwd()
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Same(t, logger, slog.Default())
	require.Equal(t, fieldManager, kube.ManagedFieldsManager)
}
