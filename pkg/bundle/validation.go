// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/logger"
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

// validateBundleForCreate applies the signature-policy validation that is only
// relevant when packages are entering a newly created bundle.
func validateBundleForCreate(b *UDSBundle) error {
	var errs []error
	if err := b.Validate(); err != nil {
		errs = append(errs, err)
	}
	for i := range b.Packages {
		pkg := &b.Packages[i]
		if err := validatePackageSignatureVerification(pkg.Name, pkg.SignatureVerification); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func validatePackageSignatureVerification(packageName string, verification *PackageSignatureVerification) error {
	if verification == nil {
		return fmt.Errorf("package %q: signature_verification block is required", packageName)
	}

	verify := true
	if verification.Verify != nil {
		verify = *verification.Verify
	}
	if verification.PublicKey != "" && strings.TrimSpace(verification.PublicKey) == "" {
		return fmt.Errorf("package %q: signature_verification.public_key must not be blank", packageName)
	}
	hasPublicKey := strings.TrimSpace(verification.PublicKey) != ""
	hasKeyless := verification.Keyless != nil

	if !verify {
		if hasPublicKey || hasKeyless {
			return fmt.Errorf("package %q: signature_verification.verify = false cannot be combined with public_key or keyless", packageName)
		}
		return nil
	}

	if hasPublicKey == hasKeyless {
		return fmt.Errorf("package %q: signature_verification must configure exactly one of public_key or keyless when verification is enabled", packageName)
	}
	if hasPublicKey {
		return nil
	}

	keyless := verification.Keyless
	hasIdentity := strings.TrimSpace(keyless.CertificateIdentity) != ""
	hasIdentityRegexp := strings.TrimSpace(keyless.CertificateIdentityRegexp) != ""
	if hasIdentity == hasIdentityRegexp {
		return fmt.Errorf("package %q: keyless verification requires exactly one of certificate_identity or certificate_identity_regexp", packageName)
	}
	if hasIdentityRegexp {
		if _, err := regexp.Compile(keyless.CertificateIdentityRegexp); err != nil {
			return fmt.Errorf("package %q: invalid certificate_identity_regexp: %w", packageName, err)
		}
	}
	hasIssuer := strings.TrimSpace(keyless.CertificateOIDCIssuer) != ""
	hasIssuerRegexp := strings.TrimSpace(keyless.CertificateOIDCIssuerRegexp) != ""
	if hasIssuer == hasIssuerRegexp {
		return fmt.Errorf("package %q: keyless verification requires exactly one of certificate_oidc_issuer or certificate_oidc_issuer_regexp", packageName)
	}
	if hasIssuerRegexp {
		if _, err := regexp.Compile(keyless.CertificateOIDCIssuerRegexp); err != nil {
			return fmt.Errorf("package %q: invalid certificate_oidc_issuer_regexp: %w", packageName, err)
		}
	}
	return nil
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
	if err := validateLogLevel(cfg.Global.LogLevel); err != nil {
		return err
	}
	return nil
}

// validateLogLevel rejects a non-empty log level that does not parse. An empty
// level is valid and means "use the default" (info).
func validateLogLevel(level string) error {
	if level == "" {
		return nil
	}
	if _, err := logger.ParseLevel(level); err != nil {
		return err
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

// Validate checks that DeployOptions is valid. Config must be non-nil and valid;
// at least one of BundlePath or Bundle must be provided.
func (o DeployOptions) Validate() error {
	if err := ValidateConfig(o.Config); err != nil {
		return err
	}
	if o.BundlePath == "" && o.Bundle == nil {
		return fmt.Errorf("at least one of BundlePath or Bundle must be provided")
	}
	return nil
}

// Validate checks that DeployPackageOptions is valid.
func (o DeployPackageOptions) Validate() error {
	if err := ValidateConfig(o.Config); err != nil {
		return err
	}
	if o.BundleDir == "" {
		return fmt.Errorf("BundleDir is required")
	}
	return nil
}

// Validate checks that RemoveOptions is valid. Config must be non-nil and valid;
// at least one of BundlePath or Bundle must be provided.
func (o RemoveOptions) Validate() error {
	if err := ValidateConfig(o.Config); err != nil {
		return err
	}
	if o.BundlePath == "" && o.Bundle == nil {
		return fmt.Errorf("at least one of BundlePath or Bundle must be provided")
	}
	return nil
}

// Validate checks that RemovePackageOptions is valid.
func (o RemovePackageOptions) Validate() error {
	return ValidateConfig(o.Config)
}

// Validate checks that CreateOptions is valid.
func (o CreateOptions) Validate() error {
	if err := ValidateConfig(o.Config); err != nil {
		return err
	}
	if o.BundleFile == "" {
		return fmt.Errorf("BundleFile is required")
	}
	return nil
}

// Validate checks that CreatePackageOptions is valid.
func (o CreatePackageOptions) Validate() error {
	if err := ValidateConfig(o.Config); err != nil {
		return err
	}
	if o.BlobDir == "" {
		return fmt.Errorf("BlobDir is required")
	}
	if o.BundleDir == "" {
		return fmt.Errorf("BundleDir is required")
	}
	return nil
}

// Validate checks that PullOptions is valid.
func (o PullOptions) Validate() error {
	return ValidateConfig(o.Config)
}

// Validate checks that PushOptions is valid.
func (o PushOptions) Validate() error {
	return ValidateConfig(o.Config)
}

// Validate checks that ReconfigureOptions is valid.
// Checks Source, DefaultsFile, and Suffix shape.
func (o ReconfigureOptions) Validate() error {
	if o.Source == "" {
		return fmt.Errorf("source is required")
	}
	if o.DefaultsFile == "" {
		return fmt.Errorf("defaults file is required")
	}
	if !validSuffix.MatchString(o.Suffix) {
		return fmt.Errorf("invalid suffix %q: must start with '-' and contain only alphanumeric characters, dots, underscores, and hyphens", o.Suffix)
	}
	if err := validateLogLevel(o.Options.LogLevel); err != nil {
		return err
	}
	return nil
}

// ValidatePackageNames checks that all names exist in the bundle's package list.
// The error message names the unknown packages and lists all available packages.
func ValidatePackageNames(names []string, packages []Package) error {
	if len(names) == 0 {
		return nil
	}
	known := make(map[string]bool, len(packages))
	knownNames := make([]string, 0, len(packages))
	for _, p := range packages {
		known[p.Name] = true
		knownNames = append(knownNames, p.Name)
	}
	sort.Strings(knownNames)
	var unknown []string
	for _, n := range names {
		if !known[n] {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown packages %v not defined in bundle (available packages: %v)", unknown, knownNames)
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
// On violation it returns a *DependencyViolationError listing, per removed
// package, the remaining packages that still depend on it.
func ValidateRemovalSafety(ctx context.Context, streams iostreams.IOStreams, b *UDSBundle, packageNames []string) error {
	if len(packageNames) == 0 {
		return nil
	}
	dag, err := BuildDependencyGraph(ctx, streams, b)
	if err != nil {
		return fmt.Errorf("failed to build dependency graph: %w", err)
	}
	blockers := dependentBlockers(dag, packageNames)
	if len(blockers) == 0 {
		return nil
	}
	return formatDependencyError("cannot remove package(s) with bundle dependents", "is required by", blockers)
}

// ValidateDeploySafety returns an error if any selected package depends on a
// package that is NOT selected. Empty packageNames (full deploy) yields nil.
// On violation it returns a *DependencyViolationError listing, per selected
// package, the dependencies that are missing from the selection.
func ValidateDeploySafety(ctx context.Context, streams iostreams.IOStreams, b *UDSBundle, packageNames []string) error {
	if len(packageNames) == 0 {
		return nil
	}
	dag, err := BuildDependencyGraph(ctx, streams, b)
	if err != nil {
		return fmt.Errorf("failed to build dependency graph: %w", err)
	}
	missing := missingDependencies(dag, packageNames)
	if len(missing) == 0 {
		return nil
	}
	return formatDependencyError("cannot deploy package(s) with unselected dependencies", "requires", missing)
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

// missingDependencies returns, for each selected package, the names of its
// direct dependencies that are NOT in the selected set.
func missingDependencies(dag *DAG, selected []string) map[string][]string {
	selSet := make(map[string]bool, len(selected))
	for _, n := range selected {
		selSet[n] = true
	}

	missing := make(map[string][]string)
	for name, deps := range dag.edges {
		if !selSet[name] {
			continue
		}
		for _, trav := range deps {
			depName := dag.traversalToName(trav)
			if !selSet[depName] {
				missing[name] = append(missing[name], depName)
			}
		}
	}

	for k := range missing {
		sort.Strings(missing[k])
	}
	return missing
}

// DependencyViolationError reports a dependency-relationship violation in a
// deploy or remove selection. Violations maps each offending package to the
// related packages that make the selection unsafe (its missing dependencies for
// deploy, or the dependents it would strand for remove). It is returned by
// ValidateDeploySafety, ValidateRemovalSafety, and the inline checks in
// DeployBundle/RemoveBundle, so library consumers can inspect the structured
// data via errors.As instead of parsing the message.
type DependencyViolationError struct {
	// Header is the summary line, e.g. "cannot deploy package(s) with unselected dependencies".
	Header string
	// Relation describes each entry's relationship, e.g. "requires" or "is required by".
	Relation string
	// Violations maps a package name to its related package names (sorted).
	Violations map[string][]string
}

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

// formatDependencyError builds a *DependencyViolationError for the given header,
// relation (e.g. "is required by" or "requires"), and violation map. It is
// returned as error for call-site convenience; consumers can recover the
// concrete type via errors.As.
func formatDependencyError(header, relation string, m map[string][]string) error {
	return &DependencyViolationError{Header: header, Relation: relation, Violations: m}
}
