// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
)

// errEmpty reports a required configuration value that was empty.
func errEmpty(name string) error {
	return fmt.Errorf("%s must not be empty", name)
}

// LocalsBlockError is a generic error when unable to parse the locals blocks
type LocalsBlockError struct {
	Diags hcl.Diagnostics
}

func (e *LocalsBlockError) Error() string {
	return fmt.Sprintf("failed to read locals block: %s", e.Diags.Error())
}

// LocalsAttributeError occurs when there is a syntactical error and a local variable is unable to be parsed
type LocalsAttributeError struct {
	Diags hcl.Diagnostics
}

func (e *LocalsAttributeError) Error() string {
	return fmt.Sprintf("failed to read locals attributes: %s", e.Diags.Error())
}

// DuplicateLocalError occurs when a local variable is declared twice in a single evaluation context
type DuplicateLocalError struct {
	Name      string
	Existing  hcl.Range
	Duplicate hcl.Range
}

func (e *DuplicateLocalError) Error() string {
	return fmt.Sprintf(
		"local.%s is already defined at %s and cannot be redefined at %s",
		e.Name,
		e.Existing,
		e.Duplicate,
	)
}

// CyclicLocalDependencyError occurs when 2 or more variables reference each other resulting in a dependency cycle
type CyclicLocalDependencyError struct {
	Cycle []string
}

func (e *CyclicLocalDependencyError) Error() string {
	return fmt.Sprintf("cyclic local dependency: %s", strings.Join(e.Cycle, " -> "))
}

// UndefinedLocalDependencyError occurs when an undefined local variable is referenced
type UndefinedLocalDependencyError struct {
	Name string
	// Position the missing variable was referenced at
	Position hcl.Range
}

func (e *UndefinedLocalDependencyError) Error() string {
	return fmt.Sprintf("undefined local dependency: %s referenced at %s", e.Name, e.Position)
}

// LocalEvaluationError occurs when a local variable is unable to be evaluated such as a call to file() with a file
// that does not exist
type LocalEvaluationError struct {
	Name  string
	Diags hcl.Diagnostics
}

func (e *LocalEvaluationError) Error() string {
	return fmt.Sprintf("failed to evaluate local %q: %s", e.Name, e.Diags.Error())
}
