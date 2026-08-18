// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package main is the entrypoint for the uds binary.
package main

import (
	"errors"
	"fmt"
	"os"

	legacycli "github.com/defenseunicorns/uds-cli/internal/legacy/cli"
	"github.com/defenseunicorns/uds-cli/internal/mode"
	"github.com/defenseunicorns/uds-cli/internal/version"
	legacyconfig "github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"helm.sh/helm/v4/pkg/kube"
)

func main() {
	// Set the Helm field manager name to match Zarf's so that resources deployed via UDS bundles
	// and resources deployed directly via Zarf are interchangeable without requiring --force-conflicts.
	kube.ManagedFieldsManager = cluster.FieldManagerName
	if err := run(mode.ProcessArgs()); err != nil {
		pterm.Error.Println(err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	selected, features, args, err := mode.Resolve(args, os.LookupEnv)
	if err != nil {
		return err
	}
	if err := os.Setenv(mode.FeaturesEnv, features.String()); err != nil {
		return err
	}
	if version.Version != "unset" {
		legacyconfig.CLIVersion = version.Version
	}
	root, err := newRootCommand(selected)
	if err != nil {
		return err
	}
	root.PersistentFlags().String("features", "", "Features, comma separated name=true or name=false pairs. CLI_FEATURES is also supported.")
	root.SetArgs(args)
	return root.Execute()
}

func newRootCommand(selected mode.Mode) (*cobra.Command, error) {
	switch selected {
	case mode.Legacy:
		return legacycli.NewRootCommand(), nil
	case mode.Next:
		return nil, errors.New("NextMode is not available in this UDS CLI release")
	default:
		return nil, fmt.Errorf("unsupported CLI mode %q", selected)
	}
}
