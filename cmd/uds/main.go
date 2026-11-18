// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package main is the entrypoint for the uds binary.
package main

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/cli"
	legacycli "github.com/defenseunicorns/uds-cli/internal/legacy/cli"
	"github.com/defenseunicorns/uds-cli/internal/mode"
	"github.com/defenseunicorns/uds-cli/internal/version"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
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
	root, err := newRootCommand(selected, features)
	if err != nil {
		return err
	}
	root.PersistentFlags().String("features", "", "Features, comma separated name, name=true, or name=false pairs. CLI_FEATURES is also supported.")
	root.SetArgs(adaptMaruWaitArgs(args))
	return root.Execute()
}

// adaptMaruWaitArgs handles Maru's "zarf tools wait-for KIND/PROTOCOL ..." callbacks.
// Zarf 1.x requires "resource" before Kubernetes kinds and "network" before TCP/HTTP/HTTPS.
// Translate at the executable boundary so both Legacy and Next receive the new syntax.
// Commands that already specify "resource" or "network" pass through unchanged.
func adaptMaruWaitArgs(args []string) []string {
	if len(args) < 4 || args[0] != "zarf" || args[1] != "tools" || args[2] != "wait-for" || strings.HasPrefix(args[3], "-") {
		return args
	}
	subcommand := "resource"
	switch args[3] {
	case "tcp", "http", "https":
		subcommand = "network"
	case "network", "resource":
		return args
	}
	return slices.Insert(slices.Clone(args), 3, subcommand)
}

func newRootCommand(selected mode.Mode, features mode.FeatureSet) (*cobra.Command, error) {
	switch selected {
	case mode.Legacy:
		return legacycli.NewRootCommand(), nil
	case mode.Next:
		streams := iostreams.New(os.Stdin, os.Stdout, os.Stderr)
		root := cli.NewRootCommand(streams, features)
		root.AddCommand(legacycli.NewRunCommand())
		return root, nil
	default:
		return nil, fmt.Errorf("unsupported CLI mode %q", selected)
	}
}
