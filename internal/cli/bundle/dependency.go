// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"
	"sort"
	"strings"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
)

const (
	bundleFileName         = bundleinternal.BundleFileName
	bundleDefaultsFileName = bundleinternal.BundleDefaultsFileName
)

func formatDependencyError(header, relation string, violations map[string][]string) error {
	names := make([]string, 0, len(violations))
	for name := range violations {
		names = append(names, name)
	}
	sort.Strings(names)

	var message strings.Builder
	fmt.Fprintf(&message, "%s:\n", header)
	for _, name := range names {
		fmt.Fprintf(&message, "  - %q %s: %s\n", name, relation, strings.Join(violations[name], ", "))
	}
	return fmt.Errorf("%s", strings.TrimRight(message.String(), "\n"))
}
