// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
)

var (
	ErrConfigRequired             = errors.New("config is required")
	ErrConfigGlobalRequired       = errors.New("config.Global is required")
	ErrConfigOptionsRequired      = errors.New("config.Options is required")
	ErrInvalidConcurrency         = errors.New("invalid concurrency")
	ErrInvalidTemporaryDirectory  = errors.New("invalid temporary directory")
	ErrReadBundleFile             = errors.New("cannot read bundle file")
	ErrParseHCL                   = errors.New("failed to parse HCL")
	ErrDecodeBundle               = errors.New("failed to decode bundle")
	ErrDecodePackageMetadata      = errors.New("failed to decode package metadata")
	ErrDecodePackageDependencies  = errors.New("failed to decode package dependencies")
	ErrReadFile                   = errors.New("reading file")
	ErrInvalidFile                = errors.New("invalid file")
	ErrUnexpectedHCLBody          = errors.New("unexpected HCL body type")
	ErrFileFunctionUnavailable    = errors.New("file function is unavailable")
	ErrReadDefaultsFile           = errors.New("cannot read defaults file")
	ErrExtractLocals              = errors.New("failed to extract locals")
	ErrEvaluateHCLExpression      = errors.New("failed to evaluate HCL expression")
	ErrReadPackageDependencies    = errors.New("failed to read depends_on")
	ErrInvalidPackageDependencies = errors.New("invalid package dependencies")
	ErrInvalidPackageReference    = errors.New("invalid package reference")
	ErrReadConfigFile             = errors.New("cannot read config file")
	ErrParseConfig                = errors.New("failed to parse config HCL")
	ErrDecodeConfig               = errors.New("failed to decode config")
	ErrReadVariables              = errors.New("failed to read variables")
	ErrEvaluateVariables          = errors.New("failed to evaluate variables")
	ErrConvertVariables           = errors.New("failed to convert variables")
	ErrInvalidVariables           = errors.New("invalid variables")
	ErrUnsupportedVariableType    = errors.New("unsupported variable type")
	ErrParseDefaults              = errors.New("failed to parse defaults HCL")
	ErrInvalidDefaults            = errors.New("invalid defaults file")
	ErrUnknownPackageDependency   = errors.New("unknown package dependency")
	ErrDependencyCycle            = errors.New("dependency cycle detected")
	ErrPackageNotInBundle         = errors.New("package is not in the bundle")
	ErrUnknownPackages            = errors.New("unknown packages")
	ErrBuildDependencyGraph       = errors.New("failed to build dependency graph")
)

var (
	_ error = (*EmptyParameterError)(nil)
	_ error = (*LocalsBlockError)(nil)
	_ error = (*LocalsAttributeError)(nil)
	_ error = (*DuplicateLocalError)(nil)
	_ error = (*CyclicLocalDependencyError)(nil)
	_ error = (*UndefinedLocalDependencyError)(nil)
	_ error = (*LocalEvaluationError)(nil)
)

type EmptyParameterError struct{ Name string }

func (e EmptyParameterError) Error() string {
	return fmt.Sprintf("%s must not be empty", e.Name)
}

// LocalsBlockError is a generic error when unable to parse the locals blocks
type LocalsBlockError struct {
	Diags hcl.Diagnostics
}

func (e *LocalsBlockError) Error() string {
	return fmt.Sprintf("failed to read locals block: %s", e.Diags.Error())
}

func (e *LocalsBlockError) Unwrap() error { return e.Diags }

// LocalsAttributeError occurs when there is a syntactical error and a local variable is unable to be parsed
type LocalsAttributeError struct {
	Diags hcl.Diagnostics
}

func (e *LocalsAttributeError) Error() string {
	return fmt.Sprintf("failed to read locals attributes: %s", e.Diags.Error())
}

func (e *LocalsAttributeError) Unwrap() error { return e.Diags }

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

func (e *LocalEvaluationError) Unwrap() error { return e.Diags }
