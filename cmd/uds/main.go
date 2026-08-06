// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package main is the entrypoint for the uds binary.
package main

import (
	"fmt"
	"os"

	legacycli "github.com/defenseunicorns/uds-cli/internal/legacy/cli"
	"github.com/defenseunicorns/uds-cli/internal/mode"
	"github.com/pterm/pterm"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"helm.sh/helm/v4/pkg/kube"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		pterm.Error.Println(err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	selectedMode, args, err := mode.Resolve(args, os.LookupEnv)
	if err != nil {
		return err
	}
	if err := os.Setenv(mode.EnvName, selectedMode.String()); err != nil {
		return err
	}

	switch selectedMode {
	case mode.Legacy:
		// Set the Helm field manager name to match Zarf's so that resources deployed via UDS bundles
		// and resources deployed directly via Zarf are interchangeable without requiring --force-conflicts.
		kube.ManagedFieldsManager = cluster.FieldManagerName
		cmd, err := legacycli.NewCommand()
		if err != nil {
			return err
		}
		cmd.SetArgs(args)
		return cmd.Execute()
	case mode.Next:
		return fmt.Errorf("CLI mode %q is not available in this build", selectedMode)
	default:
		return fmt.Errorf("unsupported CLI mode %q", selectedMode)
	}
}
