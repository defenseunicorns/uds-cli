// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package main is the entrypoint for the uds binary.
package main

import (
	"embed"

	"github.com/defenseunicorns/uds-cli/src/cmd"
	"github.com/defenseunicorns/zarf/src/pkg/packager/lint"
)

//go:embed zarf.schema.json
var zarfSchema embed.FS

func main() {
	lint.ZarfSchema = zarfSchema
	cmd.Execute()
}
