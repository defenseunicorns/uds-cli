// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"errors"
	"fmt"
)

// Validate checks that the bundle satisfies all required constraints.
func (b *UDSBundle) Validate() error {
	var errs []error

	if b.UDS.BundleAPIVersion == "" {
		errs = append(errs, fmt.Errorf("uds.bundle_api_version is required"))
	}

	if b.Metadata.Name == "" {
		errs = append(errs, fmt.Errorf("metadata.name is required"))
	}

	if len(b.Packages) == 0 {
		errs = append(errs, fmt.Errorf("at least one package is required"))
	}

	packageNames := make(map[string]bool)
	for i, pkg := range b.Packages {
		if pkg.Name == "" {
			errs = append(errs, fmt.Errorf("package[%d]: name (block label) is required", i))
		}
		if packageNames[pkg.Name] {
			errs = append(errs, fmt.Errorf("package[%d]: duplicate package name %q", i, pkg.Name))
		}
		packageNames[pkg.Name] = true

		if pkg.Source == "" {
			errs = append(errs, fmt.Errorf("package %q: source is required", pkg.Name))
		}

		for _, dep := range pkg.DependsOn {
			if dep == pkg.Name {
				errs = append(errs, fmt.Errorf("package %q: cannot depend on itself", pkg.Name))
			}
			if !containsPackage(b.Packages, dep) {
				errs = append(errs, fmt.Errorf("package %q: depends_on references unknown package %q", pkg.Name, dep))
			}
		}

		componentNames := make(map[string]bool)
		for _, comp := range pkg.OptionalComponents {
			if comp == "" {
				errs = append(errs, fmt.Errorf("package %q: optional_components contains empty string", pkg.Name))
			}
			if componentNames[comp] {
				errs = append(errs, fmt.Errorf("package %q: duplicate optional component %q", pkg.Name, comp))
			}
			componentNames[comp] = true
		}
	}

	return errors.Join(errs...)
}

func containsPackage(packages []Package, name string) bool {
	for _, p := range packages {
		if p.Name == name {
			return true
		}
	}
	return false
}
