// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package modify

import (
	"fmt"
	"os"

	"github.com/defenseunicorns/pkg/helpers/v2"
	goyaml "github.com/goccy/go-yaml"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/api/convert"
	"github.com/zarf-dev/zarf/src/api/v1beta1"
)

func writeSourceDefinition(path string, pkg api.Package) error {
	// Write only the source API version. Zarf's multi-version writer projects beta
	// service components into an alpha init package that cannot pass source validation.
	var definition any = convert.PackageToV1alpha1(pkg)
	if pkg.GetAPIVersion() == v1beta1.APIVersion {
		definition = convert.PackageToV1beta1(pkg)
	}
	contents, err := goyaml.MarshalWithOptions(
		definition,
		goyaml.IndentSequence(true),
		goyaml.UseLiteralStyleIfMultiline(true),
	)
	if err != nil {
		return fmt.Errorf("marshaling source package definition: %w", err)
	}
	return os.WriteFile(path, contents, helpers.ReadWriteUser)
}
