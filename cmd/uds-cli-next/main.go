// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package main is the entrypoint for the UDS CLI.
package main

import (
	"fmt"
	"os"

	"github.com/defenseunicorns/uds-cli/internal/cli"
	"github.com/defenseunicorns/uds-cli/internal/mode"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

func main() {
	streams := iostreams.New(os.Stdin, os.Stdout, os.Stderr)
	if err := run(mode.ProcessArgs(), os.LookupEnv, streams); err != nil {
		fmt.Fprintf(streams.ErrOut(), "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, lookupEnv func(string) (string, bool), streams iostreams.IOStreams) error {
	rootCmd, args, err := newRootCommand(args, lookupEnv, streams)
	if err != nil {
		return err
	}
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

func newRootCommand(args []string, lookupEnv func(string) (string, bool), streams iostreams.IOStreams) (*cobra.Command, []string, error) {
	_, features, args, err := mode.Resolve(args, lookupEnv)
	if err != nil {
		return nil, nil, err
	}
	features[mode.FeatureNextMode] = true

	rootCmd := cli.NewRootCommand(streams, features)
	rootCmd.PersistentFlags().String("features", "", "Features, comma separated name, name=true, or name=false pairs. CLI_FEATURES is also supported.")
	return rootCmd, args, nil
}
