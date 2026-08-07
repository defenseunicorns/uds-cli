// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrPackageNotDeployed indicates that a package is not present on the target.
var ErrPackageNotDeployed = errors.New("package not deployed")

// ErrNotImplemented indicates that an optional operation has no implementation.
var ErrNotImplemented = errors.New("not yet implemented")

// DependencyViolationError reports a dependency-relationship violation in a
// deploy or remove selection. Violations maps each offending package to the
// related packages that make the selection unsafe (its missing dependencies for
// deploy, or the dependents it would strand for remove). It is returned by
// ValidateDeploySafety and ValidateRemovalSafety, so library consumers can
// inspect the structured data via errors.As instead of parsing the message.
type DependencyViolationError struct {
	// Header is the summary line, e.g. "cannot deploy package(s) with unselected dependencies".
	Header string
	// Relation describes each entry's relationship, e.g. "requires" or "is required by".
	Relation string
	// Violations maps a package name to its related package names (sorted).
	Violations map[string][]string
}

// Error formats the dependency violations for command-line display.
func (e *DependencyViolationError) Error() string {
	names := make([]string, 0, len(e.Violations))
	for k := range e.Violations {
		names = append(names, k)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(e.Header)
	b.WriteString(":\n")
	for _, n := range names {
		fmt.Fprintf(&b, "  - %q %s: %s\n", n, e.Relation, strings.Join(e.Violations[n], ", "))
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatDependencyError builds a public dependency violation error from the
// relationship data calculated by the internal bundle model.
func formatDependencyError(header, relation string, violations map[string][]string) error {
	return &DependencyViolationError{
		Header:     header,
		Relation:   relation,
		Violations: violations,
	}
}
