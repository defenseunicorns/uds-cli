// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"fmt"
	"regexp"

	"github.com/defenseunicorns/uds-cli/src/types"
)

var packageNamespacePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]*[a-z0-9])?$`)

func validatePackageNamespace(pkg types.Package) error {
	if pkg.Namespace == "" {
		return nil
	}

	if !packageNamespacePattern.MatchString(pkg.Namespace) {
		return fmt.Errorf("invalid namespace for package %q: %q (must match Kubernetes namespace format [a-z0-9]([a-z0-9-]*[a-z0-9])?)", pkg.Name, pkg.Namespace)
	}

	return nil
}

func validatePackageNamespaces(packages []types.Package) error {
	for _, pkg := range packages {
		if err := validatePackageNamespace(pkg); err != nil {
			return err
		}
	}

	return nil
}
