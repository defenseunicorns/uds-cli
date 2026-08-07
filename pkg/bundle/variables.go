// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"strconv"
	"strings"
)

// Flatten converts top-level scalar variables to uppercase string values.
func (v Variables) Flatten() (map[string]string, error) {
	out := make(map[string]string, len(v))
	for key, value := range v {
		upper := strings.ToUpper(key)
		switch value := value.(type) {
		case string:
			out[upper] = value
		case float64:
			out[upper] = strconv.FormatFloat(value, 'f', -1, 64)
		case bool:
			out[upper] = strconv.FormatBool(value)
		}
	}
	return out, nil
}
