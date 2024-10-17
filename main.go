// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package main is the entrypoint for the uds binary.
package main

import (
	"embed"

	"github.com/defenseunicorns/uds-cli/src/cmd"
	"github.com/zarf-dev/zarf/src/pkg/lint"
)

//go:embed zarf.schema.json
var zarfSchema embed.FS

func main() {
	lint.ZarfSchema = zarfSchema
	cmd.Execute()
}
