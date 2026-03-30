// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"strconv"
	"strings"

	"github.com/zarf-dev/zarf/src/pkg/value"
)

// MergeVariables deep-merges variables from overrides into base, returning a new Variables map.  This function
// leverages Zarf's deep-merge for values.
func MergeVariables(base, overrides Variables) Variables {
	if base == nil && overrides == nil {
		return nil
	}
	result := make(Variables, len(base))
	for k, v := range base {
		if m, ok := v.(map[string]any); ok {
			result[k] = deepCopyMap(m)
		} else {
			result[k] = v
		}
	}
	value.Values(result).DeepMerge(value.Values(overrides))
	return result
}

// deepCopyMap recursively copies a map[string]any so that mutations to the copy do not affect the original.
func deepCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if nested, ok := v.(map[string]any); ok {
			out[k] = deepCopyMap(nested)
		} else {
			out[k] = v
		}
	}
	return out
}

// Flatten returns the top-level scalar values (string, float64, bool) as an
// uppercased string map suitable for Zarf's SetVariables passthrough.
// Nested objects are excluded — they are only available via template rendering.
//
// Example:
//
//	Input:  Variables{"domain": "uds.dev", "logging": Variables{"vectorEnabled": true}}
//	Output: map[string]string{"DOMAIN": "uds.dev"}
func (v Variables) Flatten() map[string]string {
	result := make(map[string]string)
	for k, val := range v {
		switch s := val.(type) {
		case string:
			result[strings.ToUpper(k)] = s
		case float64:
			result[strings.ToUpper(k)] = strconv.FormatFloat(s, 'f', -1, 64)
		case bool:
			result[strings.ToUpper(k)] = strconv.FormatBool(s)
			// nested maps are excluded
		}
	}
	return result
}
