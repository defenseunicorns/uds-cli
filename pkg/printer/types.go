// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package printer

import "io"

// Format represents an output format for structured command results.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"
)

// ResourcePrinter formats a command result object and writes it to a writer.
// Implementations handle text (human-readable), JSON, and YAML serialization.
type ResourcePrinter interface {
	PrintObj(obj any, w io.Writer) error
}
