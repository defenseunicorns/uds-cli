// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"strconv"
	"strings"
)

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
