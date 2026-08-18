// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package main is the entrypoint for the UDS CLI.
package main

import (
	"fmt"
	"os"

	"github.com/defenseunicorns/uds-cli/internal/cli"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func main() {
	streams := iostreams.New(os.Stdin, os.Stdout, os.Stderr)
	rootCmd := cli.NewRootCommand(streams)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
