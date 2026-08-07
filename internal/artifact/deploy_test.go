// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/stretchr/testify/assert"
)

// These deploy-order tests exercise the bundle → DAG → topological-levels
// path that ZarfDeployer.DeployBundle relies on. They overlap with cases in
// dag_test.go on purpose: kept here so any future change to the DAG keeps
// the deploy-side guarantees (correct ordering, level grouping) regressed
// against. Pure orchestration scheduling lives in deploy_orchestrator_test.go.

func TestApplyValuesFilesOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pkgs     []spec.Package
		override map[string][]string
		want     [][]string
	}{
		{
			name: "package in map gets override paths",
			pkgs: []spec.Package{
				{Name: "pkg-a", ValuesFiles: []string{"original-a.yaml"}},
			},
			override: map[string][]string{
				"pkg-a": {"values/0.yaml"},
			},
			want: [][]string{{"values/0.yaml"}},
		},
		{
			name: "package missing from map gets nil, not HCL fallback",
			pkgs: []spec.Package{
				{Name: "pkg-a", ValuesFiles: []string{"original-a.yaml"}},
				{Name: "pkg-b", ValuesFiles: []string{"original-b.yaml"}},
			},
			override: map[string][]string{
				"pkg-a": {"values/0.yaml"},
				// pkg-b intentionally absent
			},
			want: [][]string{{"values/0.yaml"}, nil},
		},
		{
			name: "all packages missing from map get nil",
			pkgs: []spec.Package{
				{Name: "pkg-a", ValuesFiles: []string{"original-a.yaml"}},
			},
			override: map[string][]string{},
			want:     [][]string{nil},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ApplyValuesFilesOverride(tc.pkgs, tc.override)
			for i, pkg := range tc.pkgs {
				assert.Equal(t, tc.want[i], pkg.ValuesFiles)
			}
		})
	}
}
