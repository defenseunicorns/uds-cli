// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import "github.com/defenseunicorns/uds-cli/pkg/bundle/spec"

// ApplyValuesFilesOverride replaces every package's ValuesFiles with the
// corresponding artifact path entry from override. Packages absent from the map get nil.
// An artifact deployment must never fall back to HCL-defined paths that may no
// longer exist on disk. An empty map is a valid override that sets all
// packages' ValuesFiles to nil.
func ApplyValuesFilesOverride(pkgs []spec.Package, override map[string][]string) {
	for i := range pkgs {
		pkgs[i].ValuesFiles = override[pkgs[i].Name]
	}
}
