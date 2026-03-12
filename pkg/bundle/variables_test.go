// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVariables_Flatten(t *testing.T) {
	tests := []struct {
		name string
		vars Variables
		want map[string]string
	}{
		{
			name: "nil returns empty map without panic",
			vars: nil,
			want: map[string]string{},
		},
		{
			name: "empty map returns empty map",
			vars: Variables{},
			want: map[string]string{},
		},
		{
			name: "string value",
			vars: Variables{"domain": "uds.dev"},
			want: map[string]string{"DOMAIN": "uds.dev"},
		},
		{
			name: "empty string value included",
			vars: Variables{"key": ""},
			want: map[string]string{"KEY": ""},
		},
		{
			name: "float64 converted to string",
			vars: Variables{"count": float64(10)},
			want: map[string]string{"COUNT": "10"},
		},
		{
			name: "float with decimal part",
			vars: Variables{"ratio": float64(1.5)},
			want: map[string]string{"RATIO": "1.5"},
		},
		{
			name: "negative number",
			vars: Variables{"offset": float64(-5)},
			want: map[string]string{"OFFSET": "-5"},
		},
		{
			name: "bool true",
			vars: Variables{"enabled": true},
			want: map[string]string{"ENABLED": "true"},
		},
		{
			name: "bool false",
			vars: Variables{"enabled": false},
			want: map[string]string{"ENABLED": "false"},
		},
		{
			name: "nested map excluded",
			vars: Variables{"nested": map[string]any{"k": "v"}},
			want: map[string]string{},
		},
		{
			name: "mixed scalar and nested: only scalars included",
			vars: Variables{
				"domain":  "uds.dev",
				"count":   float64(3),
				"logging": map[string]any{"level": "info"},
			},
			want: map[string]string{
				"DOMAIN": "uds.dev",
				"COUNT":  "3",
			},
		},
		{
			name: "key uppercasing: snake_case",
			vars: Variables{"cluster_name": "test"},
			want: map[string]string{"CLUSTER_NAME": "test"},
		},
		{
			name: "key uppercasing: camelCase",
			vars: Variables{"myKey": "val"},
			want: map[string]string{"MYKEY": "val"},
		},
		{
			name: "all nested: empty result",
			vars: Variables{
				"a": map[string]any{"b": "c"},
				"d": Variables{"e": "f"},
			},
			want: map[string]string{},
		},
		{
			name: "multiple scalars",
			vars: Variables{
				"host":  "example.com",
				"port":  float64(8080),
				"debug": false,
			},
			want: map[string]string{
				"HOST":  "example.com",
				"PORT":  "8080",
				"DEBUG": "false",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.vars.Flatten()
			assert.Equal(t, tt.want, got)
		})
	}
}
