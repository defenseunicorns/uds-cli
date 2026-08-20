// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	// ErrInvalidConfig occurs when bundle configuration fails validation.
	ErrInvalidConfig = errors.New("invalid bundle configuration")
	// ErrValidateDependencies occurs when bundle dependency relationships cannot be evaluated.
	ErrValidateDependencies = errors.New("validating bundle dependencies")
	// ErrBundleInputRequired occurs when neither a bundle path nor an in-memory bundle is provided.
	ErrBundleInputRequired = errors.New("bundle path or bundle is required")
	// ErrBundleDirRequired occurs when a bundle operation has no bundle directory.
	ErrBundleDirRequired = errors.New("bundle directory is required")
	// ErrTargetDirRequired occurs when a pull operation has no destination directory.
	ErrTargetDirRequired = errors.New("target directory is required")
	// ErrBundleFileRequired occurs when bundle creation has no definition file.
	ErrBundleFileRequired = errors.New("bundle file is required")
	// ErrSourceRequired occurs when an operation has no bundle source.
	ErrSourceRequired = errors.New("bundle source is required")
	// ErrInvalidOCIReference occurs when an OCI reference cannot be parsed.
	ErrInvalidOCIReference = errors.New("invalid OCI reference")
	// ErrDefaultsFileRequired occurs when reconfiguration has no defaults file.
	ErrDefaultsFileRequired = errors.New("defaults file is required")
	// ErrInvalidSuffix occurs when a reconfiguration suffix is unsafe for tags, paths, or HCL.
	ErrInvalidSuffix = errors.New("invalid reconfiguration suffix")
	// ErrInvalidSigningOptions occurs when a signing mode or its required inputs are invalid.
	ErrInvalidSigningOptions = errors.New("invalid bundle signing options")
	// ErrInvalidVerificationPolicy occurs when a signature verification policy is invalid.
	ErrInvalidVerificationPolicy = errors.New("invalid bundle verification policy")
	// ErrCreateBundle occurs when bundle creation fails after option validation.
	ErrCreateBundle = errors.New("creating bundle")
	// ErrDeployBundle occurs when bundle parsing or deployment fails.
	ErrDeployBundle = errors.New("deploying bundle")
	// ErrInspectBundle occurs when reading or verifying bundle metadata fails.
	ErrInspectBundle = errors.New("inspecting bundle")
	// ErrPrepareDeploySource occurs when an artifact cannot be prepared for deployment.
	ErrPrepareDeploySource = errors.New("preparing deploy source")
	// ErrRemoveBundle occurs when bundle parsing or removal fails.
	ErrRemoveBundle = errors.New("removing bundle")
	// ErrPullBundle occurs when a bundle cannot be pulled from OCI storage.
	ErrPullBundle = errors.New("pulling bundle")
	// ErrPushBundle occurs when bundle extraction or registry upload fails.
	ErrPushBundle = errors.New("pushing bundle")
	// ErrReconfigureBundle occurs when local or remote bundle reconfiguration fails.
	ErrReconfigureBundle = errors.New("reconfiguring bundle")
	// ErrSignBundle occurs when a validated bundle signing operation fails.
	ErrSignBundle = errors.New("signing bundle")
	// ErrVerifyBundle occurs when a validated bundle verification operation fails.
	ErrVerifyBundle = errors.New("verifying bundle")
)

// ErrBundleNotSigned indicates that a bundle has no signature evidence.
var ErrBundleNotSigned = errors.New("bundle is not signed")

var _ error = (*DependencyViolationError)(nil)

type DependencyViolationError struct {
	// Violations maps a package name to its related package names (sorted).
	Violations map[string][]string
	header     string
	relation   string
}

// Error formats the dependency violations for command-line display.
func (e *DependencyViolationError) Error() string {
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
	return &DependencyViolationError{
		header:     header,
		relation:   relation,
		Violations: violations,
	}
}
