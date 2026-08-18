// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrBundleNotSigned indicates that a bundle has no signature evidence.
var ErrBundleNotSigned = errors.New("bundle is not signed")

type dependencyViolationError struct {
	// Violations maps a package name to its related package names (sorted).
	Violations map[string][]string
	header     string
	relation   string
}

// Error formats the dependency violations for command-line display.
func (e *dependencyViolationError) Error() string {
	names := make([]string, 0, len(e.Violations))
	for k := range e.Violations {
		names = append(names, k)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(e.header)
	b.WriteString(":\n")
	for _, n := range names {
		fmt.Fprintf(&b, "  - %q %s: %s\n", n, e.relation, strings.Join(e.Violations[n], ", "))
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatDependencyError builds a public dependency violation error from the
// relationship data calculated by the internal bundle model.
func formatDependencyError(header, relation string, violations map[string][]string) error {
	return &dependencyViolationError{
		header:     header,
		relation:   relation,
		Violations: violations,
	}
}
