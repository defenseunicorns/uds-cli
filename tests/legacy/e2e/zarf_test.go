// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/utils/exec"
)

// NOTE: These tests test that the embedded `zarf` commands are imported properly and function as expected

// TestZarfLint tests to ensure that the `zarf dev lint` command functions (which requires the zarf schema to be embedded in main.go)
func TestZarfLint(t *testing.T) {
	stdout, _ := runCmd(t, "zarf dev lint testdata/legacy/packages/podinfo")
	require.Contains(t, stdout, "Image not pinned with digest - ghcr.io/stefanprodan/podinfo:")
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
			args:        args{tool: "helm", toolRepo: "helm.sh/helm/v4"},
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
			args:        args{tool: "archiver", toolRepo: "github.com/mholt/archives"},
		},
	}

	for _, tt := range tests {
		res, stderr := runCmd(t, fmt.Sprintf("zarf tools %s version", tt.args.tool))

		toolRepoVerArgs := fmt.Sprintf("list -f '{{.Version}}' -m %s", tt.args.toolRepo)
		verRes, _, verErr := exec.Cmd("go", strings.Split(toolRepoVerArgs, " ")...)
		require.NoError(t, verErr)

		toolVersion := strings.Split(verRes, "'")[1]
		output := res
		if res == "" {
			output = stderr
		}
		require.Contains(t, output, toolVersion)
	}
}

func TestZarfFeatureBootstrap(t *testing.T) {
	zarfVersion, _, err := exec.Cmd("go", "list", "-f", "{{.Version}}", "-m", "github.com/zarf-dev/zarf")
	require.NoError(t, err)
	tests := []struct {
		name       string
		args       []string
		wantOutput string
	}{
		{"equals form before Zarf", []string{"--features=NextMode=false", "zarf", "version"}, strings.TrimSpace(zarfVersion)},
		{"value form inside Zarf", []string{"zarf", "--features", "NextMode=false", "tools", "yq", "--version"}, "yq"},
		{"equals form inside tool", []string{"zarf", "tools", "kubectl", "--features=NextMode=false", "version", "--client"}, "Client Version"},
		{"Zarf feature", []string{"zarf", "--features=values=false", "version"}, strings.TrimSpace(zarfVersion)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, err := e2e.UDS(tt.args...)
			require.NoError(t, err, stdout, stderr)
			require.Contains(t, stdout+stderr, tt.wantOutput)
		})
	}
}

func TestZarfToolsIgnoreCLIFeatures(t *testing.T) {
	t.Setenv("CLI_FEATURES", "values=true")
	stdout, stderr, err := e2e.UDS("zarf", "tools", "kubectl", "version", "--client")
	require.NoError(t, err, stdout, stderr)
	require.Contains(t, stdout+stderr, "Client Version")
}
