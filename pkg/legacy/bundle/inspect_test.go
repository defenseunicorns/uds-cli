// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"testing"

	goyaml "github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api"
)

func TestInspectVariableOutput(t *testing.T) {
	data, err := goyaml.Marshal(inspectVariable{Name: "EXAMPLE"})
	require.NoError(t, err)
	require.Equal(t, "name: EXAMPLE\n", string(data))

	data, err = goyaml.Marshal(inspectVariable{
		Name: "EXAMPLE", Sensitive: true, AutoIndent: true, Pattern: "^value$",
		Type: api.FileVariableType, Description: "example variable", Default: "value", Prompt: true,
	})
	require.NoError(t, err)
	require.YAMLEq(t, `name: EXAMPLE
sensitive: true
autoIndent: true
pattern: ^value$
type: file
description: example variable
default: value
prompt: true
`, string(data))
}

// TestGatherComponentImages ensures that `uds inspect --list-images` surfaces images referenced
// via imageArchives, not just those in component.Images. Regression test for CLI-235.
func TestGatherComponentImages(t *testing.T) {
	const (
		archiveImg = "ghcr.io/uds-packages/tinkerbell/hookos-artifacts:0.1.0"
		regularImg = "ghcr.io/defenseunicorns/some-image:1.0.0"
	)

	tests := []struct {
		name       string
		components []api.Component
		want       []string
	}{
		{
			name:       "no components",
			components: nil,
			want:       nil,
		},
		{
			name: "only imageArchives",
			components: []api.Component{
				{
					Name: "hookos-artifacts",
					ImageArchives: []api.ImageArchive{
						{Path: "image.tar", Images: []string{archiveImg}},
					},
				},
			},
			want: []string{archiveImg},
		},
		{
			name: "images and imageArchives across components",
			components: []api.Component{
				{Name: "regular", Images: []api.Image{{Name: regularImg}}},
				{
					Name: "hookos-artifacts",
					ImageArchives: []api.ImageArchive{
						{Path: "image.tar", Images: []string{archiveImg}},
					},
				},
			},
			want: []string{regularImg, archiveImg},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ElementsMatch(t, tt.want, gatherComponentImages(tt.components))
		})
	}
}
