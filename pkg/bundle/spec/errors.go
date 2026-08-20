// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package spec

import (
	"errors"
	"fmt"
)

var (
	// ErrBundleAPIVersionRequired occurs when a bundle omits uds.bundle_api_version.
	ErrBundleAPIVersionRequired = errors.New("uds.bundle_api_version is required")
	// ErrMetadataNameRequired occurs when a bundle omits metadata.name.
	ErrMetadataNameRequired = errors.New("metadata.name is required")
	// ErrPackagesRequired occurs when a bundle contains no packages.
	ErrPackagesRequired = errors.New("at least one package is required")
)

var (
	_ error = (*UnsupportedBundleAPIVersionError)(nil)
	_ error = (*PackageNameRequiredError)(nil)
	_ error = (*InvalidPackageNameError)(nil)
	_ error = (*DuplicatePackageNameError)(nil)
	_ error = (*PackageSourceRequiredError)(nil)
	_ error = (*SelfDependencyError)(nil)
	_ error = (*UnknownDependencyError)(nil)
	_ error = (*EmptyOptionalComponentError)(nil)
	_ error = (*DuplicateOptionalComponentError)(nil)
)

// UnsupportedBundleAPIVersionError occurs when a bundle uses an API version other than the supported version.
type UnsupportedBundleAPIVersionError struct {
	Actual   string
	Expected string
}

func (e *UnsupportedBundleAPIVersionError) Error() string {
	return fmt.Sprintf("uds.bundle_api_version %q is not supported; expected %q", e.Actual, e.Expected)
}

// PackageNameRequiredError occurs when a package block omits its name label.
type PackageNameRequiredError struct {
	Index int
}

func (e *PackageNameRequiredError) Error() string {
	return fmt.Sprintf("package[%d]: name (block label) is required", e.Index)
}

// InvalidPackageNameError occurs when a package name contains a path separator or is a dot path.
type InvalidPackageNameError struct {
	Index int
	Name  string
}

func (e *InvalidPackageNameError) Error() string {
	return fmt.Sprintf("package[%d]: name %q must not contain path separators or be a dot path", e.Index, e.Name)
}

// DuplicatePackageNameError occurs when multiple package blocks use the same name.
type DuplicatePackageNameError struct {
	Index int
	Name  string
}

func (e *DuplicatePackageNameError) Error() string {
	return fmt.Sprintf("package[%d]: duplicate package name %q", e.Index, e.Name)
}

// PackageSourceRequiredError occurs when a package omits its source.
type PackageSourceRequiredError struct {
	Package string
}

func (e *PackageSourceRequiredError) Error() string {
	return fmt.Sprintf("package %q: source is required", e.Package)
}

// SelfDependencyError occurs when a package declares itself as a dependency.
type SelfDependencyError struct {
	Package string
}

func (e *SelfDependencyError) Error() string {
	return fmt.Sprintf("package %q: cannot depend on itself", e.Package)
}

// UnknownDependencyError occurs when a package references a dependency absent from the bundle.
type UnknownDependencyError struct {
	Package    string
	Dependency string
}

func (e *UnknownDependencyError) Error() string {
	return fmt.Sprintf("package %q: depends_on references unknown package %q", e.Package, e.Dependency)
}

// EmptyOptionalComponentError occurs when a package contains an empty optional component name.
type EmptyOptionalComponentError struct {
	Package string
}

func (e *EmptyOptionalComponentError) Error() string {
	return fmt.Sprintf("package %q: optional_components contains empty string", e.Package)
}

// DuplicateOptionalComponentError occurs when a package repeats an optional component name.
type DuplicateOptionalComponentError struct {
	Package   string
	Component string
}

func (e *DuplicateOptionalComponentError) Error() string {
	return fmt.Sprintf("package %q: duplicate optional component %q", e.Package, e.Component)
}
