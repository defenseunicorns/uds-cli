// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package spec

import (
	"errors"
	"strings"
)

// Validate checks that the bundle satisfies all required constraints.
func (b *UDSBundle) Validate() error {
	var errs []error

	const supportedAPIVersion = "uds.dev/v1alpha1"
	if b.UDS.BundleAPIVersion == "" {
		errs = append(errs, ErrBundleAPIVersionRequired)
	} else if b.UDS.BundleAPIVersion != supportedAPIVersion {
		errs = append(errs, &UnsupportedBundleAPIVersionError{Actual: b.UDS.BundleAPIVersion, Expected: supportedAPIVersion})
	}

	if b.Metadata.Name == "" {
		errs = append(errs, ErrMetadataNameRequired)
	}

	if len(b.Packages) == 0 {
		errs = append(errs, ErrPackagesRequired)
	}

	packageNames := make(map[string]bool, len(b.Packages))
	for i, pkg := range b.Packages {
		if pkg.Name == "" {
			errs = append(errs, &PackageNameRequiredError{Index: i})
		}
		if strings.ContainsAny(pkg.Name, "/\\") || pkg.Name == "." || pkg.Name == ".." {
			errs = append(errs, &InvalidPackageNameError{Index: i, Name: pkg.Name})
		}
		if packageNames[pkg.Name] {
			errs = append(errs, &DuplicatePackageNameError{Index: i, Name: pkg.Name})
		}
		packageNames[pkg.Name] = true

		if pkg.Source == "" {
			errs = append(errs, &PackageSourceRequiredError{Package: pkg.Name})
		}

		for _, dep := range pkg.DependsOn {
			if dep.Name == pkg.Name {
				errs = append(errs, &SelfDependencyError{Package: pkg.Name})
			}
			if !containsPackage(b.Packages, dep.Name) {
				errs = append(errs, &UnknownDependencyError{Package: pkg.Name, Dependency: dep.Name})
			}
		}

		componentNames := make(map[string]bool, len(pkg.OptionalComponents))
		for _, comp := range pkg.OptionalComponents {
			if comp == "" {
				errs = append(errs, &EmptyOptionalComponentError{Package: pkg.Name})
			}
			if componentNames[comp] {
				errs = append(errs, &DuplicateOptionalComponentError{Package: pkg.Name, Component: comp})
			}
			componentNames[comp] = true
		}
	}

	return errors.Join(errs...)
}

// containsPackage reports whether the package set contains the named package.
func containsPackage(packages []Package, name string) bool {
	for _, pkg := range packages {
		if pkg.Name == name {
			return true
		}
	}
	return false
}
