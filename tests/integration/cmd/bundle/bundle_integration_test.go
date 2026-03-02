// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package bundle_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/pkg/cmd/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestInspectCommand_Integration(t *testing.T) {
	bundlePath := testDataPath(t, "bundles/spec-compliant/bundle.uds.hcl")

	streams, _, out, _ := iostreams.NewTestIOStreams()

	cmd := bundle.NewInspectCommand(streams)
	cmd.SetArgs([]string{bundlePath})

	err := cmd.Execute()
	require.NoError(t, err)

	output := out.String()

	// Verify BUNDLE METADATA section
	assert.Contains(t, output, "BUNDLE METADATA")
	assert.Contains(t, output, "Name:        uds-core-example")
	assert.Contains(t, output, "Version:     0.1.0")

	// Verify PACKAGES section
	assert.Contains(t, output, "PACKAGES (3)")

	// Verify locals were fully resolved
	assert.NotContains(t, output, "${local.")

	// Verify resolved source URLs
	assert.Contains(t, output, "oci://ghcr.io/defenseunicorns/packages/uds/core-base:0.59.1-upstream")

	// Verify depends_on and valuesFiles
	assert.Contains(t, output, "DependsOn: core_base")
	assert.Contains(t, output, "DependsOn: core_base, core_logging")
	assert.Contains(t, output, "Value Files: values/loki.yaml, values/vector.yaml")
	assert.Contains(t, output, "Value Files: values/monitoring.yaml")
	assert.Contains(t, output, "Namespace: monitoring")
}

// Note: Error cases (OCI reference, file not found) are tested in unit tests
// because CheckErr calls os.Exit(1) which would terminate the test process.

func TestDeployCommand_Integration(t *testing.T) {
	bundlePath := testDataPath(t, "bundles/deploy/init")

	streams, in, out, _ := iostreams.NewTestIOStreams()
	// Simulate user declining the deployment
	in.WriteString("n\n")

	cmd := bundle.NewDeployCommand(streams)
	cmd.SetArgs([]string{bundlePath})

	err := cmd.Execute()
	require.NoError(t, err)

	output := out.String()

	// Verify bundle metadata is displayed
	assert.Contains(t, output, "BUNDLE METADATA")
	assert.Contains(t, output, "Name:        k3d-core-init")

	// Verify packages are displayed
	assert.Contains(t, output, "PACKAGES (2)")
	assert.Contains(t, output, "uds_k3d_dev")
	assert.Contains(t, output, "init")

	// Verify confirmation prompt was shown
	assert.Contains(t, output, "Deploy this bundle?")
	assert.Contains(t, output, "Deployment cancelled")
}

func TestDeployCommand_WithBundleFile_Integration(t *testing.T) {
	bundlePath := testDataPath(t, "bundles/deploy/init/bundle.uds.hcl")

	streams, in, out, _ := iostreams.NewTestIOStreams()
	// Simulate user declining the deployment
	in.WriteString("n\n")

	cmd := bundle.NewDeployCommand(streams)
	cmd.SetArgs([]string{bundlePath})

	err := cmd.Execute()
	require.NoError(t, err)

	output := out.String()

	// Verify bundle metadata is displayed
	assert.Contains(t, output, "Name:        k3d-core-init")
	// Verify confirmation prompt was shown
	assert.Contains(t, output, "Deploy this bundle?")
}

func TestPullCommand_Integration(t *testing.T) {
	streams, _, out, _ := iostreams.NewTestIOStreams()

	cmd := bundle.NewPullCommand(streams)
	cmd.SetArgs([]string{"ghcr.io/test:v1"})

	err := cmd.Execute()
	require.NoError(t, err)

	assert.Contains(t, out.String(), "Bundle pulled successfully")
}

func TestPushCommand_Integration(t *testing.T) {
	streams, _, out, _ := iostreams.NewTestIOStreams()

	cmd := bundle.NewPushCommand(streams)
	cmd.SetArgs([]string{"ghcr.io/test:v1"})

	err := cmd.Execute()
	require.NoError(t, err)

	assert.Contains(t, out.String(), "Bundle pushed successfully")
}

func TestRemoveCommand_Integration(t *testing.T) {
	streams, _, out, _ := iostreams.NewTestIOStreams()

	cmd := bundle.NewRemoveCommand(streams)
	cmd.SetArgs([]string{"ghcr.io/test:v1"})

	err := cmd.Execute()
	require.NoError(t, err)

	assert.Contains(t, out.String(), "Bundle removed successfully")
}
