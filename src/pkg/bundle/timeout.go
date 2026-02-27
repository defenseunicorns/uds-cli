// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"fmt"
	"strings"
	"time"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
)

func resolvePackageTimeout(pkg types.Package) (time.Duration, error) {
	timeoutString := strings.TrimSpace(pkg.Timeout)
	if timeoutString == "" {
		return config.HelmTimeout, nil
	}

	timeout, err := time.ParseDuration(timeoutString)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout for package %q: %q (use duration format like 30s, 10m, or 1h30m)", pkg.Name, pkg.Timeout)
	}

	if timeout <= 0 {
		return 0, fmt.Errorf("invalid timeout for package %q: %q (timeout must be greater than zero)", pkg.Name, pkg.Timeout)
	}

	return timeout, nil
}

func validatePackageTimeouts(packages []types.Package) error {
	for _, pkg := range packages {
		if _, err := resolvePackageTimeout(pkg); err != nil {
			return err
		}
	}

	return nil
}
