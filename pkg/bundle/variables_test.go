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

func TestMergeVariables(t *testing.T) {
	tests := []struct {
		name      string
		base      Variables
		overrides Variables
		want      Variables
	}{
		{
			name:      "both nil returns nil",
			base:      nil,
			overrides: nil,
			want:      nil,
		},
		{
			name:      "nil overrides returns copy of base",
			base:      Variables{"a": "a-base-value", "b": 1},
			overrides: nil,
			want:      Variables{"a": "a-base-value", "b": 1},
		},
		{
			name:      "nil base returns copy of overrides",
			base:      nil,
			overrides: Variables{"a": "a-override-value", "b": 2},
			want:      Variables{"a": "a-override-value", "b": 2},
		},
		{
			name:      "overrides takes precedence at leaf level",
			base:      Variables{"a": "a-base-value", "b": 1},
			overrides: Variables{"a": "a-override-value", "b": 2},
			want:      Variables{"a": "a-override-value", "b": 2},
		},
		{
			name: "nested maps are deep-merged",
			base: Variables{
				"feature": map[string]any{
					"auth":    true,
					"logging": false,
				},
			},
			overrides: Variables{
				"feature": map[string]any{
					"auth": false,
				},
			},
			want: Variables{
				"feature": map[string]any{
					"auth":    false,
					"logging": false,
				},
			},
		},
		{
			name:      "overrides takes precedence when types mismatched",
			base:      Variables{"a": map[string]any{"b": true}},
			overrides: Variables{"a": "a-override-value"},
			want:      Variables{"a": "a-override-value"},
		},
		{
			name:      "unmatched keys are preserved from both base and overrides",
			base:      Variables{"a": "a-base-value", "b": 1},
			overrides: Variables{"c": "c-override-value", "d": 2},
			want:      Variables{"a": "a-base-value", "b": 1, "c": "c-override-value", "d": 2},
		},
		{
			name: "deeply nested merge",
			base: Variables{
				"a0": map[string]any{
					"a1": map[string]any{
						"a2": "a3-base-value",
						"b2": "b3-base-value",
					},
				},
			},
			overrides: Variables{
				"a0": map[string]any{
					"a1": map[string]any{
						"b2": "b3-override-value",
					},
				},
			},
			want: Variables{
				"a0": map[string]any{
					"a1": map[string]any{
						"a2": "a3-base-value",
						"b2": "b3-override-value",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeVariables(tt.base, tt.overrides)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMergeVariables_DoesNotMutateBase(t *testing.T) {
	base := Variables{"a": "a-base-value"}
	overrides := Variables{"a": "a-override-value"}

	_ = MergeVariables(base, overrides)

	assert.Equal(t, "a-base-value", base["a"], "base must not be mutated")
}

func TestMergeVariables_DoesNotMutateBaseNestedMap(t *testing.T) {
	base := Variables{"a": map[string]any{"b": true}}
	overrides := Variables{"a": map[string]any{"b": false}}

	_ = MergeVariables(base, overrides)

	a := base["a"].(map[string]any)
	assert.Equal(t, true, a["b"], "base nested map must not be mutated")
}
