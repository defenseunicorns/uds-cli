// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"
	"strconv"
	"strings"
)

// MergeVariables deep-merges variables from overrides into base, returning a new Variables map.
// The base is deep-copied so callers can mutate the result without affecting their inputs.
// Nested Variables are deep-merged; everything else (scalars, lists) is replaced wholesale,
// matching Helm overlay conventions.
func MergeVariables(base, overrides Variables) Variables {
	if base == nil && overrides == nil {
		return nil
	}
	result := deepCopyVariables(base)
	if result == nil {
		result = make(Variables)
	}
	deepMerge(result, overrides)
	return result
}

// deepCopyVariables returns a deep copy of v. nil maps copy to nil.
func deepCopyVariables(v Variables) Variables {
	if v == nil {
		return nil
	}
	out := make(Variables, len(v))
	for k, val := range v {
		out[k] = deepCopyAny(val)
	}
	return out
}

// deepCopySlice returns a deep copy of s. nil slices copy to nil.
func deepCopySlice(s []any) []any {
	if s == nil {
		return nil
	}
	out := make([]any, len(s))
	for i, val := range s {
		out[i] = deepCopyAny(val)
	}
	return out
}

// deepCopyAny dispatches on the dynamic type of an element drawn from a Variables
// or []any. Scalars are value-typed and shared safely.
//
// The HCL parser only produces Variables for nested objects (never bare
// map[string]any). A bare map[string]any here means a non-HCL caller violated
// that contract: aliasing it would silently propagate the bad shape into
// merge/flatten/templating, so we panic at the copy site to surface the bug
// at its origin rather than letting it tunnel further downstream.
func deepCopyAny(v any) any {
	switch t := v.(type) {
	case Variables:
		return deepCopyVariables(t)
	case []any:
		return deepCopySlice(t)
	case map[string]any:
		panic(fmt.Sprintf("bare map[string]any in Variables (key contract violation); use Variables — got %v", t))
	default:
		return t
	}
}

// deepMerge recursively merges src into dst. Nested Variables are deep-merged;
// any other type (incl. []any) is replaced wholesale (Helm overlay convention).
// dst is mutated in place; src is not modified, and src-side nested maps/slices
// are deep-copied so the result never aliases src.
func deepMerge(dst, src Variables) {
	for k, sv := range src {
		if dv, exists := dst[k]; exists {
			if dm, dOK := dv.(Variables); dOK {
				if sm, sOK := sv.(Variables); sOK {
					deepMerge(dm, sm)
					continue
				}
			}
		}
		dst[k] = deepCopyAny(sv)
	}
}

// Flatten returns the top-level values as an uppercased string map suitable for
// Zarf's SetVariables passthrough. Only scalar types (string, float64, bool) are
// included; non-scalar values (lists, nested Variables, other types) are silently
// skipped and must be passed to Zarf via values_files instead.
//
// Complex types are excluded because values_files is the proper channel for them.
// Templates already handle nested Variables natively, and skipping non-scalars
// steers authors toward the values_files path rather than introducing a footgun
// where chart authors would need to JSON-decode Zarf var tokens.
//
// Example:
//
//	Input:  Variables{"domain": "uds.dev", "ports": []any{1.0, 2.0}, "nested": Variables{"key": "val"}}
//	Output: map[string]string{"DOMAIN": "uds.dev"}  (ports and nested skipped)
func (v Variables) Flatten() (map[string]string, error) {
	out := make(map[string]string, len(v))
	for k, val := range v {
		upper := strings.ToUpper(k)
		switch s := val.(type) {
		case string:
			out[upper] = s
		case float64:
			out[upper] = strconv.FormatFloat(s, 'f', -1, 64)
		case bool:
			out[upper] = strconv.FormatBool(s)
		default:
			// Skip non-scalar types ([]any, Variables, etc.) silently.
			// They belong in values_files where templates can process them natively.
		}
	}
	return out, nil
}
