// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/defenseunicorns/zarf/src/pkg/utils/exec"
	"github.com/stretchr/testify/require"
)

// NOTE: These tests test that the embedded `zarf` commands are imported properly and function as expected

// TestZarfLint tests to ensure that the `zarf dev lint` command functions (which requires the zarf schema to be embedded in main.go)
func TestZarfLint(t *testing.T) {
	cmd := strings.Split("zarf dev lint src/test/packages/podinfo", " ")
	_, stdErr, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	require.Contains(t, stdErr, "Image not pinned with digest - ghcr.io/stefanprodan/podinfo:6.4.0")
}

func TestZarfToolsVersions(t *testing.T) {
	type args struct {
		tool     string
		toolRepo string
	}
	tests := []struct {
		name        string
		description string
		args        args
	}{
		{
			name:        "HelmVersion",
			description: "zarf tools helm version",
			args:        args{tool: "helm", toolRepo: "helm.sh/helm/v3"},
		},
		{
			name:        "CraneVersion",
			description: "zarf tools crane version",
			args:        args{tool: "crane", toolRepo: "github.com/google/go-containerregistry"},
		},
		{
			name:        "SyftVersion",
			description: "zarf tools syft version",
			args:        args{tool: "syft", toolRepo: "github.com/anchore/syft"},
		},
		{
			name:        "ArchiverVersion",
			description: "zarf tools archiver version",
			args:        args{tool: "archiver", toolRepo: "github.com/mholt/archiver/v3"},
		},
	}

	for _, tt := range tests {
		cmdArgs := fmt.Sprintf("zarf tools %s version", tt.args.tool)
		res, stdErr, err := e2e.UDS(strings.Split(cmdArgs, " ")...)
		require.NoError(t, err)

		toolRepoVerArgs := fmt.Sprintf("list -f '{{.Version}}' -m %s", tt.args.toolRepo)
		verRes, _, verErr := exec.Cmd("go", strings.Split(toolRepoVerArgs, " ")...)
		require.NoError(t, verErr)

		toolVersion := strings.Split(verRes, "'")[1]
		output := res
		if res == "" {
			output = stdErr
		}
		require.Contains(t, output, toolVersion)
	}
}
