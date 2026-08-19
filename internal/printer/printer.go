// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package printer

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// Format represents an output format.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"
)

// ResourcePrinter formats a result object.
type ResourcePrinter interface{ PrintObj(any, io.Writer) error }

// NewPrinter returns a ResourcePrinter for the given format.
func NewPrinter(format Format) (ResourcePrinter, error) {
	switch format {
	case FormatText:
		return &TextPrinter{}, nil
	case FormatJSON:
		return &JSONPrinter{}, nil
	case FormatYAML:
		return &YAMLPrinter{}, nil
	default:
		return nil, fmt.Errorf("unsupported output format: %q (valid: text, json, yaml)", format)
	}
}

// ParseFormat converts a string to a Format, returning an error for unknown values.
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(s) {
	case "text", "":
		return FormatText, nil
	case "json":
		return FormatJSON, nil
	case "yaml":
		return FormatYAML, nil
	default:
		return "", fmt.Errorf("unknown output format %q, valid values are: text, json, yaml", s)
	}
}

var _ ResourcePrinter = &JSONPrinter{}

// JSONPrinter formats result objects as indented JSON.
type JSONPrinter struct{}

// PrintObj writes an object as indented JSON.
func (p *JSONPrinter) PrintObj(obj any, w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(obj)
}

var _ ResourcePrinter = &YAMLPrinter{}

// YAMLPrinter formats result objects as YAML.
type YAMLPrinter struct{}

// PrintObj writes an object as YAML.
func (p *YAMLPrinter) PrintObj(obj any, w io.Writer) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(obj); err != nil {
		return err
	}
	return enc.Close()
}
