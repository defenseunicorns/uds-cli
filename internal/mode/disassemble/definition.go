// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import (
	"fmt"
	"os"

	"github.com/defenseunicorns/pkg/helpers/v2"
	goyaml "github.com/goccy/go-yaml"
)

func writeSourceDefinition(path string, definition any) error {
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
