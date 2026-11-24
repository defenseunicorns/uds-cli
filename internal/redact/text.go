// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package redact masks known sensitive values in diagnostic text.
package redact

import (
	"io"
	"sort"
	"strconv"
	"strings"
)

// Text builds a reusable filter for non-empty values.
func Text(secrets []string) *strings.Replacer {
	var values []string
	for _, value := range secrets {
		if value == "" {
			continue
		}
		values = append(values, value)
		// Paths and other option values may be quoted in diagnostics.
		quoted := strconv.Quote(value)
		if escaped := quoted[1 : len(quoted)-1]; escaped != value {
			values = append(values, escaped)
		}
	}
	// Match longer secrets first so a shared prefix does not expose their suffixes.
	sort.Slice(values, func(i, j int) bool {
		return len(values[i]) > len(values[j])
	})
	pairs := make([]string, 0, 2*len(values))
	for _, secret := range values {
		pairs = append(pairs, secret, "***")
	}
	return strings.NewReplacer(pairs...)
}

type textReplacer interface{ Replace(string) string }

// Writer masks matching text and reports the number of input bytes consumed.
func Writer(next io.Writer, replacer textReplacer) io.Writer { return writer{next, replacer} }

type writer struct {
	next     io.Writer
	replacer textReplacer
}

func (w writer) Write(p []byte) (int, error) {
	if _, err := io.WriteString(w.next, w.replacer.Replace(string(p))); err != nil {
		return 0, err
	}
	// io.Writer counts consumed input, even when redaction shortens the output.
	return len(p), nil
}
