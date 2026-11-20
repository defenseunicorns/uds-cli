// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package analysis

import (
	"bytes"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/defenseunicorns/uds-cli/internal/bundle"
)

var templatePosition = regexp.MustCompile(`^:(\d+)(?::(\d+))?:`)

func (d document) checkTemplate(variables bundle.Variables, execute bool) *Diagnostic {
	name := filepath.Base(d.path)
	tmpl, err := bundle.ParseValuesTemplate(name, string(d.text))
	message := "invalid values template syntax"
	if err == nil && execute {
		err = tmpl.Execute(io.Discard, map[string]any{"vars": variables})
		message = "values template evaluation failed; supply required variables through defaults, --config, or --set, or use --skip-template-evaluation"
	}
	if err == nil {
		return nil
	}
	result := Diagnostic{Path: d.path, Message: message}
	// Go templates expose positions in error text, which may also contain
	// evaluated secrets. Extract only coordinates, never publish the raw error.
	match := templatePosition.FindStringSubmatch(strings.TrimPrefix(err.Error(), "template: "+name))
	if match == nil {
		return &result
	}
	line, _ := strconv.Atoi(match[1])
	sourceLine := d.text
	for range line - 1 {
		_, next, ok := bytes.Cut(sourceLine, []byte{'\n'})
		if !ok {
			return &result
		}
		sourceLine = next
	}
	result.Line = line
	if match[2] != "" {
		lineText, _, _ := bytes.Cut(sourceLine, []byte{'\n'})
		column, _ := strconv.Atoi(match[2])
		result.Column = utf8.RuneCount(lineText[:min(column, len(lineText))]) + 1
	}
	return &result
}
