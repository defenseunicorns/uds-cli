// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Validate checks that the bundle satisfies all required constraints.
func (b *UDSBundle) Validate() error {
	var errs []error

	const supportedAPIVersion = "uds.dev/v1alpha1"
	if b.UDS.BundleAPIVersion == "" {
		errs = append(errs, fmt.Errorf("uds.bundle_api_version is required"))
	} else if b.UDS.BundleAPIVersion != supportedAPIVersion {
		errs = append(errs, fmt.Errorf("uds.bundle_api_version %q is not supported; expected %q", b.UDS.BundleAPIVersion, supportedAPIVersion))
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
		if strings.ContainsAny(pkg.Name, "/\\") || pkg.Name == "." || pkg.Name == ".." {
			errs = append(errs, fmt.Errorf("package[%d]: name %q must not contain path separators or be a dot path", i, pkg.Name))
		}
		if packageNames[pkg.Name] {
			errs = append(errs, fmt.Errorf("package[%d]: duplicate package name %q", i, pkg.Name))
		}
		packageNames[pkg.Name] = true

		if pkg.Source == "" {
			errs = append(errs, fmt.Errorf("package %q: source is required", pkg.Name))
		}

		for _, dep := range pkg.DependsOn {
			if dep.Name == pkg.Name {
				errs = append(errs, fmt.Errorf("package %q: cannot depend on itself", pkg.Name))
			}
			if !containsPackage(b.Packages, dep.Name) {
				errs = append(errs, fmt.Errorf("package %q: depends_on references unknown package %q", pkg.Name, dep.Name))
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

// ValidateConfig is the single entry point for validating a fully-resolved
// UDSBundleConfig. It runs nil-checks on the structure and then delegates
// field-level checks to focused sub-validators.
//
// Call this once at the boundary where config is produced (e.g. from the
// ConfigResolver). Downstream consumers should trust the config and skip
// re-validation.
func ValidateConfig(cfg *UDSBundleConfig) error {
	if err := validateConfigStructure(cfg); err != nil {
		return err
	}
	return validateOptions(cfg.Options)
}

// validateConfigStructure asserts that cfg and its required sub-structs are non-nil.
func validateConfigStructure(cfg *UDSBundleConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is required (UDSBundleConfig must not be nil)")
	}
	if cfg.Global == nil {
		return fmt.Errorf("config.Global is required (GlobalOptions must not be nil)")
	}
	if cfg.Options == nil {
		return fmt.Errorf("config.Options is required (ConfigOptions must not be nil)")
	}
	return nil
}

// validateOptions validates ConfigOptions field invariants.
func validateOptions(opts *ConfigOptions) error {
	if err := validateConcurrency(opts.Concurrency); err != nil {
		return err
	}
	return validateTmpDir(opts.TmpDir)
}

// validateConcurrency enforces the [1, MaxConcurrency] range.
func validateConcurrency(concurrency int) error {
	if concurrency < 1 {
		return fmt.Errorf("concurrency must be >= 1, got %d", concurrency)
	}
	if concurrency > MaxConcurrency {
		return fmt.Errorf("concurrency must be <= %d, got %d", MaxConcurrency, concurrency)
	}
	return nil
}

// validateTmpDir asserts that, when set, TmpDir refers to an existing directory.
// An empty value is valid and means "use the OS default".
func validateTmpDir(path string) error {
	if path == "" {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("tmp_dir: directory does not exist: %s", path)
		}
		return fmt.Errorf("tmp_dir: failed to stat directory %s: %w", path, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("tmp_dir: path is not a directory: %s", path)
	}
	return nil
}

// ValidatePackageNames checks that all names exist in the bundle's package list.
func ValidatePackageNames(names []string, packages []Package) error {
	if len(names) == 0 {
		return nil
	}
	known := make(map[string]bool, len(packages))
	for _, p := range packages {
		known[p.Name] = true
	}
	var unknown []string
	for _, n := range names {
		if !known[n] {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown packages: %v", unknown)
	}
	return nil
}

func containsPackage(packages []Package, name string) bool {
	for _, p := range packages {
		if p.Name == name {
			return true
		}
	}
	return false
}

// ValidateRemovalSafety returns an error if removing the named packages would
// leave any remaining bundle package with a missing dependency. Empty
// packageNames (full bundle removal) or no offending dependents both yield nil.
// The error message instructs the user to use --force to override.
func ValidateRemovalSafety(b *UDSBundle, packageNames []string) error {
	if len(packageNames) == 0 {
		return nil
	}
	dag, err := BuildDependencyGraph(b)
	if err != nil {
		return fmt.Errorf("failed to build dependency graph: %w", err)
	}
	blockers := dependentBlockers(dag, packageNames)
	if len(blockers) == 0 {
		return nil
	}
	return formatBlockersError(blockers)
}

// dependentBlockers returns, for each package being removed, the names of
// other bundle packages that depend on it but are NOT themselves being removed.
// Only direct dependents are reported; transitive impacts surface through the
// chain (if B depends on A and C depends on B, removing A flags B; the user can
// re-run after deciding what to do about B).
func dependentBlockers(dag *DAG, removeNames []string) map[string][]string {
	removeSet := make(map[string]bool, len(removeNames))
	for _, n := range removeNames {
		removeSet[n] = true
	}

	blockers := make(map[string][]string)
	for name, deps := range dag.edges {
		if removeSet[name] {
			continue
		}
		for _, trav := range deps {
			depName := dag.traversalToName(trav)
			if removeSet[depName] {
				blockers[depName] = append(blockers[depName], name)
			}
		}
	}

	for k := range blockers {
		sort.Strings(blockers[k])
	}
	return blockers
}

// formatBlockersError produces the user-facing error returned by
// ValidateRemovalSafety when at least one dependent would be left stranded.
func formatBlockersError(blockers map[string][]string) error {
	names := make([]string, 0, len(blockers))
	for k := range blockers {
		names = append(names, k)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("cannot remove package(s) with bundle dependents:\n")
	for _, n := range names {
		fmt.Fprintf(&b, "  - %q is required by: %s\n", n, strings.Join(blockers[n], ", "))
	}
	b.WriteString("re-run with --force to override")
	return errors.New(b.String())
}
