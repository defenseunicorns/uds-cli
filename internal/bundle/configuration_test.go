// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVariables_Flatten(t *testing.T) {
	tests := []struct {
		name    string
		vars    Variables
		want    map[string]string
		wantErr bool
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
			name: "nested Variables silently skipped",
			vars: Variables{"obj": Variables{"a": float64(1)}},
			want: map[string]string{},
		},
		{
			name: "list of primitives silently skipped",
			vars: Variables{"ports": []any{float64(1), float64(2)}},
			want: map[string]string{},
		},
		{
			name: "list of objects silently skipped",
			vars: Variables{"ports": []any{
				Variables{"name": "a", "port": float64(80)},
				Variables{"name": "b", "port": float64(90)},
			}},
			want: map[string]string{},
		},
		{
			name: "empty list silently skipped",
			vars: Variables{"x": []any{}},
			want: map[string]string{},
		},
		{
			name: "empty Variables value silently skipped",
			vars: Variables{"x": Variables{}},
			want: map[string]string{},
		},
		{
			name: "bare map[string]any silently skipped (non-scalar)",
			vars: Variables{"x": map[string]any{"k": "v"}},
			want: map[string]string{},
		},
		{
			name: "int silently skipped (non-scalar)",
			vars: Variables{"x": 7},
			want: map[string]string{},
		},
		{
			name: "mixed scalars and collections, only scalars included",
			vars: Variables{
				"domain": "uds.dev",
				"ports":  []any{float64(80)},
				"obj":    Variables{"a": float64(1)},
			},
			want: map[string]string{
				"DOMAIN": "uds.dev",
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
			name: "non-marshalable in list silently skipped",
			vars: Variables{"k": []any{make(chan int)}},
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.vars.Flatten()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "flatten ")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDeepCopyVariables(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		assert.Nil(t, deepCopyVariables(nil))
	})

	t.Run("empty map", func(t *testing.T) {
		got := deepCopyVariables(Variables{})
		require.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("flat scalars are copied", func(t *testing.T) {
		src := Variables{"s": "x", "n": float64(1), "b": true}
		got := deepCopyVariables(src)
		got["s"] = "mutated"
		assert.Equal(t, "x", src["s"], "source must not be mutated")
	})

	t.Run("nested Variables not aliased", func(t *testing.T) {
		src := Variables{"a": Variables{"b": float64(1)}}
		got := deepCopyVariables(src)
		got["a"].(Variables)["b"] = float64(99)
		assert.InDelta(t, float64(1), src["a"].(Variables)["b"], 0.001)
	})

	t.Run("nested slice not aliased", func(t *testing.T) {
		src := Variables{"a": []any{float64(1), float64(2)}}
		got := deepCopyVariables(src)
		got["a"].([]any)[0] = float64(99)
		assert.InDelta(t, float64(1), src["a"].([]any)[0], 0.001)
	})

	t.Run("Variables inside slice not aliased", func(t *testing.T) {
		src := Variables{"a": []any{Variables{"k": "v"}}}
		got := deepCopyVariables(src)
		got["a"].([]any)[0].(Variables)["k"] = "mutated"
		assert.Equal(t, "v", src["a"].([]any)[0].(Variables)["k"])
	})

	t.Run("three-deep mix", func(t *testing.T) {
		src := Variables{"a": Variables{"b": []any{Variables{"c": []any{float64(1)}}}}}
		got := deepCopyVariables(src)
		got["a"].(Variables)["b"].([]any)[0].(Variables)["c"].([]any)[0] = float64(99)
		assert.InDelta(t, float64(1),
			src["a"].(Variables)["b"].([]any)[0].(Variables)["c"].([]any)[0],
			0.001)
	})
}

func TestDeepCopySlice(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		assert.Nil(t, deepCopySlice(nil))
	})

	t.Run("empty returns non-nil empty", func(t *testing.T) {
		got := deepCopySlice([]any{})
		require.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("primitives copied", func(t *testing.T) {
		src := []any{"a", float64(1), true}
		got := deepCopySlice(src)
		got[0] = "mutated"
		assert.Equal(t, "a", src[0])
	})
}

func TestDeepMerge(t *testing.T) {
	t.Run("disjoint keys", func(t *testing.T) {
		dst := Variables{"a": float64(1)}
		src := Variables{"b": float64(2)}
		deepMerge(dst, src)
		assert.Equal(t, Variables{"a": float64(1), "b": float64(2)}, dst)
	})

	t.Run("scalar override", func(t *testing.T) {
		dst := Variables{"a": float64(1)}
		src := Variables{"a": float64(2)}
		deepMerge(dst, src)
		assert.Equal(t, Variables{"a": float64(2)}, dst)
	})

	t.Run("nested deep merge", func(t *testing.T) {
		dst := Variables{"x": Variables{"a": float64(1), "b": float64(2)}}
		src := Variables{"x": Variables{"a": float64(9)}}
		deepMerge(dst, src)
		assert.Equal(t,
			Variables{"x": Variables{"a": float64(9), "b": float64(2)}},
			dst)
	})

	t.Run("list replace not concat", func(t *testing.T) {
		dst := Variables{"p": []any{float64(1), float64(2)}}
		src := Variables{"p": []any{float64(3)}}
		deepMerge(dst, src)
		assert.Equal(t, Variables{"p": []any{float64(3)}}, dst)
	})

	t.Run("type swap map-to-scalar", func(t *testing.T) {
		dst := Variables{"x": Variables{"a": float64(1)}}
		src := Variables{"x": "replaced"}
		deepMerge(dst, src)
		assert.Equal(t, Variables{"x": "replaced"}, dst)
	})

	t.Run("type swap scalar-to-map", func(t *testing.T) {
		dst := Variables{"x": "old"}
		src := Variables{"x": Variables{"a": float64(1)}}
		deepMerge(dst, src)
		assert.Equal(t, Variables{"x": Variables{"a": float64(1)}}, dst)
	})

	t.Run("does not mutate src", func(t *testing.T) {
		dst := Variables{"a": Variables{"x": float64(1)}}
		src := Variables{"a": Variables{"y": float64(2)}}
		deepMerge(dst, src)
		assert.Equal(t, Variables{"a": Variables{"y": float64(2)}}, src,
			"src must not be mutated")
	})

	t.Run("bare map[string]any nested panics in deepCopyAny", func(t *testing.T) {
		// Variables-only invariant: a bare map[string]any value is a contract
		// violation. deepMerge calls deepCopyAny(sv), which panics rather than
		// silently aliasing or silently fixing the bad shape.
		dst := Variables{"x": Variables{"a": float64(1)}}
		src := Variables{"x": map[string]any{"b": float64(2)}}
		assert.PanicsWithValue(t,
			"bare map[string]any in Variables (key contract violation); use Variables — got map[b:2]",
			func() { deepMerge(dst, src) })
	})
}

// TestMergeVariables_PanicsOnBareMapStringAny pins the Variables-only invariant:
// a non-HCL caller stuffing a bare map[string]any into a Variables hits a loud
// panic at the deep-copy site instead of silently aliasing through merge.
func TestMergeVariables_PanicsOnBareMapStringAny(t *testing.T) {
	base := Variables{"x": map[string]any{"a": float64(1)}}
	overrides := Variables{"y": "ok"}
	assert.Panics(t, func() { _ = MergeVariables(base, overrides) })
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
			base:      Variables{"a": "a-base-value", "b": float64(1)},
			overrides: nil,
			want:      Variables{"a": "a-base-value", "b": float64(1)},
		},
		{
			name:      "nil base + overrides set",
			base:      nil,
			overrides: Variables{"a": "a-override-value"},
			want:      Variables{"a": "a-override-value"},
		},
		{
			name:      "overrides takes precedence at leaf level",
			base:      Variables{"a": "a-base-value", "b": float64(1)},
			overrides: Variables{"a": "a-override-value", "b": float64(2)},
			want:      Variables{"a": "a-override-value", "b": float64(2)},
		},
		{
			name: "nested Variables deep-merged",
			base: Variables{
				"feature": Variables{
					"auth":    true,
					"logging": false,
				},
			},
			overrides: Variables{
				"feature": Variables{
					"auth": false,
				},
			},
			want: Variables{
				"feature": Variables{
					"auth":    false,
					"logging": false,
				},
			},
		},
		{
			name:      "type swap: nested map replaced by scalar",
			base:      Variables{"a": Variables{"b": true}},
			overrides: Variables{"a": "a-override-value"},
			want:      Variables{"a": "a-override-value"},
		},
		{
			name:      "unmatched keys preserved from both sides",
			base:      Variables{"a": "a-base-value", "b": float64(1)},
			overrides: Variables{"c": "c-override-value", "d": float64(2)},
			want: Variables{
				"a": "a-base-value",
				"b": float64(1),
				"c": "c-override-value",
				"d": float64(2),
			},
		},
		{
			name: "deeply nested merge",
			base: Variables{
				"a0": Variables{
					"a1": Variables{
						"a2": "a3-base-value",
						"b2": "b3-base-value",
					},
				},
			},
			overrides: Variables{
				"a0": Variables{
					"a1": Variables{
						"b2": "b3-override-value",
					},
				},
			},
			want: Variables{
				"a0": Variables{
					"a1": Variables{
						"a2": "a3-base-value",
						"b2": "b3-override-value",
					},
				},
			},
		},
		{
			name:      "list replace not concat",
			base:      Variables{"ports": []any{float64(1), float64(2)}},
			overrides: Variables{"ports": []any{float64(3)}},
			want:      Variables{"ports": []any{float64(3)}},
		},
		{
			name:      "list retained when override absent",
			base:      Variables{"ports": []any{float64(1), float64(2)}},
			overrides: Variables{},
			want:      Variables{"ports": []any{float64(1), float64(2)}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeVariables(tt.base, tt.overrides)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMergeVariables_DoesNotAliasList(t *testing.T) {
	base := Variables{"ports": []any{float64(1), float64(2)}}
	overrides := Variables{"ports": []any{float64(3)}}

	result := MergeVariables(base, overrides)

	// Mutate the result and verify base is untouched.
	result["ports"].([]any)[0] = float64(99)
	baseList := base["ports"].([]any)
	assert.InDelta(t, float64(1), baseList[0], 0.001)
}

func TestMergeVariables_DoesNotAliasNestedObjectInList(t *testing.T) {
	base := Variables{"p": []any{Variables{"n": "a"}}}
	overrides := Variables{"q": float64(1)}

	result := MergeVariables(base, overrides)

	// Mutate a nested object inside the list in result; base must be unchanged.
	result["p"].([]any)[0].(Variables)["n"] = "mutated"
	assert.Equal(t, "a", base["p"].([]any)[0].(Variables)["n"])
}

// TestMergeVariables_DoesNotAliasOverridesList covers the symmetrical case to
// TestMergeVariables_DoesNotAliasList: src-side aliasing was the bug found in
// brutal-review (deepMerge previously did `dst[k] = sv` which shared the slice
// header with overrides). The result must own its own copy of every src-side
// nested map/slice.
func TestMergeVariables_DoesNotAliasOverridesList(t *testing.T) {
	overrides := Variables{"ports": []any{float64(1), float64(2)}}

	result := MergeVariables(nil, overrides)

	result["ports"].([]any)[0] = float64(99)
	overridesList := overrides["ports"].([]any)
	assert.InDelta(t, float64(1), overridesList[0], 0.001,
		"overrides must not be mutated by writes to the merged result")
}

func TestMergeVariables_DoesNotAliasOverridesNestedMap(t *testing.T) {
	overrides := Variables{"feature": Variables{"enabled": true}}

	result := MergeVariables(nil, overrides)

	result["feature"].(Variables)["enabled"] = false
	assert.Equal(t, true, overrides["feature"].(Variables)["enabled"],
		"overrides nested Variables must not be mutated by writes to the merged result")
}

func TestMergeVariables_DoesNotAliasOverridesNestedMapInList(t *testing.T) {
	overrides := Variables{"p": []any{Variables{"n": "a"}}}

	result := MergeVariables(nil, overrides)

	result["p"].([]any)[0].(Variables)["n"] = "mutated"
	assert.Equal(t, "a", overrides["p"].([]any)[0].(Variables)["n"],
		"overrides must own its nested objects after merge")
}

func TestMergeVariables_DoesNotMutateBase(t *testing.T) {
	base := Variables{"a": "a-base-value"}
	overrides := Variables{"a": "a-override-value"}

	_ = MergeVariables(base, overrides)

	assert.Equal(t, "a-base-value", base["a"])
}

func TestMergeVariables_DoesNotMutateBaseNestedMap(t *testing.T) {
	base := Variables{"a": Variables{"b": true}}
	overrides := Variables{"a": Variables{"b": false}}

	_ = MergeVariables(base, overrides)

	a := base["a"].(Variables)
	assert.Equal(t, true, a["b"])
}
